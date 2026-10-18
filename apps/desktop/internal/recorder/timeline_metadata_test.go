package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestTimelineKeepsCaptureOffsetsAndRecordingSessions(t *testing.T) {
	e, _, _, root := testEngine(t, false, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var start time.Time
	if err := e.call(func() error {
		start = e.segment.start
		return e.appendSpeech(&e.audioStream, asr.Speech{Start: 2 * audio.SampleRate, Samples: make([]float32, audio.SampleRate)})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.HistoryChanges():
	case <-time.After(time.Second):
		t.Fatal("speech did not finish")
	}
	first := History([]string{root})[0]
	if first.SessionID == "" || len(first.TranscriptChunks) != 1 {
		t.Fatalf("timeline metadata missing: %+v", first)
	}
	chunk := first.TranscriptChunks[0]
	if chunk.StartedAt != start.Add(2*time.Second).Format(time.RFC3339Nano) || chunk.EndedAt != start.Add(3*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("VAD capture offsets were replaced by file or inference times: %+v", chunk)
	}
	settings := e.Settings()
	settings.Recording.OutputDir = filepath.Join(root, "rotated")
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512)}) }); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	rotated := History([]string{root})
	if len(rotated) != 2 || rotated[0].SessionID != first.SessionID || rotated[1].SessionID != first.SessionID {
		t.Fatalf("file rotation split a recording session: %+v", rotated)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512)}) }); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	sessions := map[string]int{}
	for _, item := range History([]string{root}) {
		sessions[item.SessionID]++
	}
	if len(sessions) != 2 || sessions[first.SessionID] != 2 || sessions[""] != 0 {
		t.Fatalf("stop/start did not establish a separate recording session: %+v", sessions)
	}
}

func TestBatchPreservesCaptureIdentityAndDiscardsStaleChunks(t *testing.T) {
	e, _, _, root := testEngine(t, true, &fakeRuntime{})
	path := filepath.Join(root, "daybee_20261009_110000.wav")
	if err := os.WriteFile(path, []byte("fake runtime reads fixed samples"), 0600); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(root, "daybee_20261009_110000.json")
	metadata := map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-09T11:00:00+08:00", "recording_session_id": "original-session",
		"text": "旧文本", "transcript_chunks": []protocol.TranscriptChunk{{Text: "旧文本", StartedAt: "2026-10-09T11:00:02+08:00", EndedAt: "2026-10-09T11:00:03+08:00"}},
	}
	if err := config.WriteJSON(jsonPath, metadata); err != nil {
		t.Fatal(err)
	}
	if len(History([]string{root})[0].TranscriptChunks) != 1 {
		t.Fatal("valid capture chunks were not loaded")
	}
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	item := History([]string{root})[0]
	if item.StartedAt != metadata["segment_start_time"] || item.SessionID != "original-session" || len(item.TranscriptChunks) != 0 || item.TextPreview != "固定测试文本" {
		t.Fatalf("reprocessing changed capture identity or reused stale speech: %+v", item)
	}
	raw, _ := json.Marshal(metadata["transcript_chunks"])
	if len(readTranscriptChunks(raw, "编辑后的全文")) != 0 {
		t.Fatal("stale chunks replaced edited full text")
	}
}
