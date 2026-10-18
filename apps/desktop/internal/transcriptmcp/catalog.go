package transcriptmcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

const indexURI = "daybee://transcripts"

var errTranscriptNotFound = errors.New("transcript not found; use list_transcripts or search_transcripts to get a current ID")

type HistoryReader func(context.Context) ([]protocol.RecordingHistoryItem, error)

type Document struct {
	ID          string                     `json:"id"`
	URI         string                     `json:"uri"`
	StartedAt   string                     `json:"startedAt"`
	AudioSource protocol.AudioSource       `json:"audioSource"`
	AudioPath   string                     `json:"audioPath"`
	SessionID   string                     `json:"sessionId"`
	InputDevice *string                    `json:"inputDevice,omitempty"`
	Text        string                     `json:"text"`
	Chunks      []protocol.TranscriptChunk `json:"chunks"`
}

type Summary struct {
	ID          string               `json:"id"`
	URI         string               `json:"uri"`
	StartedAt   string               `json:"startedAt"`
	AudioSource protocol.AudioSource `json:"audioSource"`
	SessionID   string               `json:"sessionId"`
	Characters  int                  `json:"characters"`
	Preview     string               `json:"preview"`
}

type ListInput struct {
	Date   string `json:"date,omitempty" jsonschema:"Optional recording date in local YYYY-MM-DD format"`
	Source string `json:"source,omitempty" jsonschema:"Optional audio source: microphone or system"`
	Offset int    `json:"offset,omitempty" jsonschema:"Zero-based result offset, default 0"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Results per page, default 20, maximum 100"`
}

type SearchInput struct {
	Query  string `json:"query" jsonschema:"Required literal text to find in transcripts, case insensitive"`
	Date   string `json:"date,omitempty" jsonschema:"Optional recording date in local YYYY-MM-DD format"`
	Source string `json:"source,omitempty" jsonschema:"Optional audio source: microphone or system"`
	Offset int    `json:"offset,omitempty" jsonschema:"Zero-based result offset, default 0"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Results per page, default 20, maximum 100"`
}

type Page struct {
	Items      []Summary `json:"items"`
	Total      int       `json:"total"`
	NextOffset *int      `json:"nextOffset,omitempty"`
}

type catalog struct{ read HistoryReader }

func (c catalog) documents(ctx context.Context) ([]Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := c.read(ctx)
	if err != nil {
		return nil, err
	}
	documents := []Document{}
	seen := map[string]bool{}
	for _, item := range items {
		text := item.TextPreview
		if strings.TrimSpace(text) == "" {
			parts := make([]string, 0, len(item.TranscriptChunks))
			for _, chunk := range item.TranscriptChunks {
				parts = append(parts, chunk.Text)
			}
			text = strings.Join(parts, "\n")
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		// Finalizing WAV to FLAC keeps the same document ID.
		path := filepath.Clean(item.AudioPath)
		hash := sha256.Sum256([]byte(strings.TrimSuffix(path, filepath.Ext(path))))
		id := hex.EncodeToString(hash[:])
		if seen[id] {
			continue
		}
		seen[id] = true
		chunks := append([]protocol.TranscriptChunk{}, item.TranscriptChunks...)
		documents = append(documents, Document{ID: id, URI: indexURI + "/" + id, StartedAt: item.StartedAt,
			AudioSource: item.AudioSource, AudioPath: item.AudioPath, SessionID: item.SessionID,
			InputDevice: item.InputDevice, Text: text, Chunks: chunks})
	}
	sort.Slice(documents, func(i, j int) bool {
		a, b := recordedAt(documents[i].StartedAt), recordedAt(documents[j].StartedAt)
		if a.Equal(b) {
			return documents[i].ID < documents[j].ID
		}
		return a.After(b)
	})
	return documents, ctx.Err()
}

func (c catalog) page(ctx context.Context, input ListInput, query string) (Page, error) {
	result := Page{Items: []Summary{}}
	if input.Offset < 0 || input.Limit < 0 || input.Limit > 100 {
		return result, errors.New("offset must be nonnegative; limit must be between 1 and 100 (or omitted)")
	}
	if input.Date != "" {
		if _, err := time.Parse("2006-01-02", input.Date); err != nil {
			return result, errors.New("date must be a valid YYYY-MM-DD recording date")
		}
	}
	if input.Source != "" && input.Source != "microphone" && input.Source != "system" {
		return result, errors.New("source must be microphone or system")
	}
	documents, err := c.documents(ctx)
	if err != nil {
		return result, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 20
	}
	term := strings.ToLower(strings.TrimSpace(query))
	for _, doc := range documents {
		if input.Source != "" && string(doc.AudioSource) != input.Source {
			continue
		}
		if input.Date != "" && recordedAt(doc.StartedAt).In(time.Local).Format("2006-01-02") != input.Date {
			continue
		}
		if term != "" && !strings.Contains(strings.ToLower(doc.Text), term) {
			continue
		}
		position := result.Total
		result.Total++
		if position < input.Offset || len(result.Items) >= limit {
			continue
		}
		result.Items = append(result.Items, Summary{ID: doc.ID, URI: doc.URI, StartedAt: doc.StartedAt,
			AudioSource: doc.AudioSource, SessionID: doc.SessionID, Characters: utf8.RuneCountInString(doc.Text),
			Preview: preview(doc.Text, term)})
	}
	if len(result.Items) > 0 && input.Offset+len(result.Items) < result.Total {
		next := input.Offset + len(result.Items)
		result.NextOffset = &next
	}
	return result, nil
}

func (c catalog) document(ctx context.Context, id string) (Document, error) {
	documents, err := c.documents(ctx)
	if err != nil {
		return Document{}, err
	}
	for _, doc := range documents {
		if doc.ID == id {
			return doc, nil
		}
	}
	return Document{}, errTranscriptNotFound
}

func recordedAt(value string) time.Time {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at
	}
	at, _ := time.ParseInLocation("2006-01-02T15:04:05", value, time.Local)
	return at
}

func preview(text, term string) string {
	runes := []rune(text)
	start := 0
	if term != "" {
		lower := strings.ToLower(text)
		if index := strings.Index(lower, term); index >= 0 {
			start = max(0, utf8.RuneCountInString(lower[:index])-60)
		}
	}
	end := min(len(runes), start+240)
	value := string(runes[start:end])
	if start > 0 {
		value = "…" + value
	}
	if end < len(runes) {
		value += "…"
	}
	return value
}
