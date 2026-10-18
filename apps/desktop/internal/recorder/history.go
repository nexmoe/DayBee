package recorder

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

var recordingDate = regexp.MustCompile(`(\d{8})_(\d{6})`)

func AudioFiles(roots []string) ([]string, error) {
	files := []string{}
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			ext := strings.ToLower(filepath.Ext(path))
			if entry.Type().IsRegular() && (ext == ".wav" || ext == ".flac") && !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func historyCandidate(path string) protocol.RecordingHistoryItem {
	item := protocol.RecordingHistoryItem{AudioSource: protocol.SourceMicrophone, AudioPath: path, FolderPath: filepath.Dir(path), ID: path,
		Status: "audio_only", TranscriptChunks: []protocol.TranscriptChunk{}}
	match := recordingDate.FindStringSubmatch(filepath.Base(path))
	if len(match) > 0 {
		if at, err := time.ParseInLocation("20060102_150405", match[1]+"_"+match[2], time.Local); err == nil {
			item.StartedAt = at.Format("2006-01-02T15:04:05")
		}
	}
	if item.StartedAt == "" {
		if stat, err := os.Stat(path); err == nil {
			item.StartedAt = stat.ModTime().Format(time.RFC3339)
		}
	}
	return item
}

func History(roots []string) []protocol.RecordingHistoryItem {
	candidates := map[string]protocol.RecordingHistoryItem{}
	metadata := []string{}
	key := func(path string) string { return strings.TrimSuffix(path, filepath.Ext(path)) }
	for _, root := range roots {
		if root == "" {
			continue
		}
		root, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".wav", ".flac":
				candidates[key(path)] = historyCandidate(path)
			case ".json":
				metadata = append(metadata, path)
			}
			return nil
		})
	}
	for _, path := range metadata {
		var data map[string]json.RawMessage
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &data) != nil {
			continue
		}
		value := func(key string) string { var s string; _ = json.Unmarshal(data[key], &s); return s }
		audioPath := value("audio_path")
		candidateKey := key(path)
		if audioPath != "" {
			candidateKey = key(audioPath)
		}
		item, exists := candidates[candidateKey]
		if !exists {
			if audioPath == "" {
				continue
			}
			item = historyCandidate(audioPath)
		}
		if audioPath != "" {
			item.AudioPath = audioPath
			item.ID = audioPath
			item.FolderPath = filepath.Dir(audioPath)
		}
		if s := value("segment_start_time"); s != "" {
			item.StartedAt = s
		} else if s := value("created_at"); s != "" && !recordingDate.MatchString(filepath.Base(item.AudioPath)) {
			item.StartedAt = s
		}
		if s := value("status"); s != "" {
			item.Status = s
		}
		if s := value("input_device"); s != "" {
			item.InputDevice = &s
		}
		if source := value("audio_source"); source != "" {
			item.AudioSource = protocol.NormalizeAudioSource(protocol.AudioSource(source))
		}
		if _, hasText := data["text"]; hasText {
			item.TextPreview = strings.TrimSpace(value("text"))
			item.TranscriptChunks = readTranscriptChunks(data["transcript_chunks"], item.TextPreview)
		}
		if sessionID := value("recording_session_id"); sessionID != "" {
			item.SessionID = sessionID
		}
		if ranges, ok := data["captured_ranges"]; ok {
			item.CapturedRanges = readCapturedRanges(ranges)
		}
		candidates[candidateKey] = item
	}
	result := []protocol.RecordingHistoryItem{}
	for _, item := range candidates {
		if item.AudioPath != "" && item.StartedAt != "" {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].StartedAt == result[j].StartedAt {
			return result[i].ID < result[j].ID
		}
		return result[i].StartedAt > result[j].StartedAt
	})
	return result
}
