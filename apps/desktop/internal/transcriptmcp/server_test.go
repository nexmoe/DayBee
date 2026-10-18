package transcriptmcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func connectClient(t *testing.T, read HistoryReader) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	a, b := mcp.NewInMemoryTransports()
	server, err := NewServer("test", read).Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	if actual := client.InitializeResult().ProtocolVersion; actual != protocolVersion {
		t.Fatalf("negotiated protocol %q, want %q", actual, protocolVersion)
	}
	return client, ctx
}

func call[T any](t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args any) T {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("%s: %v, result=%+v", name, err, result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output T
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestMCPListsSearchesAndReadsCompleteTranscriptsAndResources(t *testing.T) {
	full := strings.Repeat("保留完整文稿🙂。", 200) + "Project [v2] .* 的决定。\n最后一行。"
	items := []protocol.RecordingHistoryItem{
		{AudioPath: filepath.Join(t.TempDir(), "earlier.flac"), StartedAt: "2026-10-09T10:00:00+09:00", AudioSource: protocol.SourceMicrophone, TextPreview: "Earlier speech"},
		{AudioPath: filepath.Join(t.TempDir(), "later.wav"), StartedAt: "2026-10-09T04:00:00Z", AudioSource: protocol.SourceSystem, TextPreview: full,
			TranscriptChunks: []protocol.TranscriptChunk{{Text: full, StartedAt: "2026-10-09T04:00:00Z", EndedAt: "2026-10-09T04:00:10Z", VoiceIdentity: "me"}}},
		{AudioPath: filepath.Join(t.TempDir(), "audio-only.wav"), StartedAt: "2026-10-10T00:00:00Z"},
	}
	client, ctx := connectClient(t, func(context.Context) ([]protocol.RecordingHistoryItem, error) { return items, nil })
	tools, err := client.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("tools=%+v err=%v", tools, err)
	}
	for _, tool := range tools.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool is not annotated as local and read-only: %+v", tool)
		}
	}
	page := call[Page](t, ctx, client, "list_transcripts", map[string]any{"limit": 1})
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].StartedAt != items[1].StartedAt || page.NextOffset == nil || *page.NextOffset != 1 {
		t.Fatalf("unexpected chronological page: %+v", page)
	}
	second := call[Page](t, ctx, client, "list_transcripts", map[string]any{"offset": *page.NextOffset, "limit": 1})
	if len(second.Items) != 1 || second.Items[0].StartedAt != items[0].StartedAt || second.NextOffset != nil {
		t.Fatalf("second page=%+v", second)
	}
	search := call[Page](t, ctx, client, "search_transcripts", map[string]any{"query": "PROJECT [v2] .*", "source": "system"})
	if search.Total != 1 || !strings.Contains(search.Items[0].Preview, "Project [v2] .*") || len([]rune(search.Items[0].Preview)) > 242 {
		t.Fatalf("search=%+v", search)
	}
	id := search.Items[0].ID
	read := call[Document](t, ctx, client, "read_transcript", map[string]any{"id": id})
	if read.Text != full || read.Chunks[0].VoiceIdentity != "me" || read.Chunks[0].EndedAt != items[1].TranscriptChunks[0].EndedAt {
		t.Fatal("full text or speaker timestamps were lost", read)
	}
	resources, err := client.ListResources(ctx, nil)
	if err != nil || len(resources.Resources) != 1 || resources.Resources[0].URI != indexURI {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	for _, uri := range []string{indexURI, read.URI} {
		resource, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil || len(resource.Contents) != 1 {
			t.Fatalf("read resource %s: %+v %v", uri, resource, err)
		}
		if uri == read.URI {
			var document Document
			if err := json.Unmarshal([]byte(resource.Contents[0].Text), &document); err != nil || document.Text != full {
				t.Fatal("resource truncated text", err)
			}
		}
	}
	// WAV finalization must not invalidate an ID that an assistant just listed.
	items[1].AudioPath = strings.TrimSuffix(items[1].AudioPath, ".wav") + ".flac"
	items[1].TextPreview += "新完成的语音。"
	updated := call[Document](t, ctx, client, "read_transcript", map[string]any{"id": id})
	if !strings.HasSuffix(updated.Text, "新完成的语音。") || updated.ID != id {
		t.Fatal("fresh transcript or stable ID missing")
	}
}

func TestMCPRejectsInvalidArgumentsAndArbitraryPaths(t *testing.T) {
	client, ctx := connectClient(t, func(context.Context) ([]protocol.RecordingHistoryItem, error) { return nil, nil })
	for _, input := range []struct {
		name string
		args map[string]any
	}{
		{"search_transcripts", map[string]any{"query": "  "}},
		{"search_transcripts", map[string]any{}},
		{"list_transcripts", map[string]any{"offset": -1}},
		{"list_transcripts", map[string]any{"limit": 101}},
		{"list_transcripts", map[string]any{"date": "2026-02-30"}},
		{"list_transcripts", map[string]any{"source": "other"}},
		{"read_transcript", map[string]any{"id": "../../desktop-settings.json"}},
	} {
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: input.name, Arguments: input.args})
		if err == nil && !result.IsError {
			t.Fatalf("invalid request was accepted: %+v", input)
		}
	}
	page := call[Page](t, ctx, client, "list_transcripts", map[string]any{})
	if page.Total != 0 || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty library=%+v", page)
	}
	if _, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: indexURI + "/unknown"}); err == nil {
		t.Fatal("unknown resource was accepted")
	}
}

func TestMCPFiltersByLocalDateAndKeepsChunkOnlyTranscripts(t *testing.T) {
	at := time.Date(2026, 10, 9, 10, 0, 0, 0, time.Local)
	c := catalog{read: func(context.Context) ([]protocol.RecordingHistoryItem, error) {
		return []protocol.RecordingHistoryItem{
			{AudioPath: "chunks.flac", StartedAt: at.Format(time.RFC3339), AudioSource: protocol.SourceMicrophone,
				TranscriptChunks: []protocol.TranscriptChunk{{Text: "chunk only"}}},
			{AudioPath: "system.flac", StartedAt: at.AddDate(0, 0, -1).Format(time.RFC3339), AudioSource: protocol.SourceSystem, TextPreview: "another day"},
		}, nil
	}}
	page, err := c.page(context.Background(), ListInput{Date: "2026-10-09", Source: "microphone"}, "")
	if err != nil || page.Total != 1 || page.Items[0].Preview != "chunk only" {
		t.Fatalf("filtered page=%+v err=%v", page, err)
	}
	c.read = func(context.Context) ([]protocol.RecordingHistoryItem, error) {
		return nil, errors.New("unreadable library")
	}
	if _, err := c.page(context.Background(), ListInput{}, ""); err == nil {
		t.Fatal("read failure hidden as an empty library")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.page(ctx, ListInput{}, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was ignored", err)
	}
}
