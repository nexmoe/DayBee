package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestAudioOnlyRetainsCaptureRangesAcrossSilenceAndBatchTranscription(t *testing.T) {
	e, _, assets, root := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var start time.Time
	if err := e.call(func() error {
		start = e.segment.start
		for _, offset := range []int{2, 100} {
			if err := e.appendSpeech(&e.audioStream, asr.Speech{Start: offset * audio.SampleRate, Samples: make([]float32, audio.SampleRate)}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	history := History([]string{root})
	if len(history) != 1 || history[0].TextPreview != "" || history[0].Status != "audio_only" || len(history[0].CapturedRanges) != 2 {
		t.Fatalf("audio-only scene metadata missing: %+v", history)
	}
	ranges := history[0].CapturedRanges
	if ranges[0].StartedAt != start.Add(2*time.Second).Format(time.RFC3339Nano) ||
		ranges[1].EndedAt != start.Add(101*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("capture time was replaced by VAD-compacted audio time: %+v", ranges)
	}
	if assets.asrRequested {
		t.Fatal("saving scene timing must not prepare ASR")
	}
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	updated := History([]string{root})
	if len(updated) != 1 || !reflect.DeepEqual(updated[0].CapturedRanges, ranges) || updated[0].TextPreview != "固定测试文本" {
		t.Fatalf("batch reprocessing lost scene timing: %+v", updated)
	}
}

func TestSceneSidecarDoesNotOverwriteTranscriptChunksOrCreateDuplicateHistory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "daybee_20261009_110000.flac")
	chunks := []protocol.TranscriptChunk{{Text: "会议内容", StartedAt: "2026-10-09T11:00:02Z", EndedAt: "2026-10-09T11:00:03Z"}}
	if err := config.WriteJSON(strings.TrimSuffix(path, ".flac")+".json", map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-09T11:00:00Z", "text": "会议内容", "status": "ok", "transcript_chunks": chunks,
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteJSON(strings.TrimSuffix(path, ".flac")+".recording.json", map[string]any{
		"audio_path": path, "recording_session_id": "session", "captured_ranges": []protocol.RecordingTimeRange{
			{StartedAt: chunks[0].StartedAt, EndedAt: chunks[0].EndedAt},
		},
	}); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root})
	if len(items) != 1 || !reflect.DeepEqual(items[0].TranscriptChunks, chunks) || len(items[0].CapturedRanges) != 1 || items[0].Status != "ok" {
		t.Fatalf("scene sidecar changed transcript history: %+v", items)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fixture unexpectedly created audio")
	}
	if ranges := readCapturedRanges(json.RawMessage(`[{"startedAt":"2026-10-09T11:00:05Z","endedAt":"2026-10-09T11:00:02Z"}]`)); len(ranges) != 0 {
		t.Fatal("backwards capture range was accepted")
	}
}
