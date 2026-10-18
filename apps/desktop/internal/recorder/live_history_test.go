package recorder

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestRecognizedSpeechIsVisibleBeforeRecordingStops(t *testing.T) {
	e, _, _, root := testEngine(t, false, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 8; i++ {
		if err := e.call(func() error {
			return e.consume(audio.Packet{Samples: make([]float32, 512), DeviceLabel: "Test Mic"})
		}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-e.HistoryChanges():
		case <-time.After(time.Second):
			t.Fatal("completed recognition did not notify the timeline")
		}
		// The timeline reads durable metadata, not the five-item live preview.
		history := History([]string{root})
		if len(history) != 1 || strings.Count(history[0].TextPreview, "固定测试文本") != i {
			t.Fatalf("completed speech missing from the timeline while recording: %+v", history)
		}
		if history[0].SessionID == "" || len(history[0].TranscriptChunks) != i {
			t.Fatalf("live capture timing was truncated or missing: %+v", history[0])
		}
		if !e.Status().Recording {
			t.Fatal("streaming results stopped the recording")
		}
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	history := History([]string{root})
	if len(history) != 1 || strings.Count(history[0].TextPreview, "固定测试文本") != 8 {
		t.Fatalf("finalizing duplicated or lost streamed results: %+v", history)
	}
}

func TestLiveTranscriptSurvivesFLACFinalization(t *testing.T) {
	root := t.TempDir()
	settings := protocol.DefaultSettings(root)
	settings.Recording.AudioFormat = protocol.FLAC
	settings.Recording.AutoSwitchDevice = false
	e := New(settings, &fakeCapture{}, nativeTestAssets{&fakeAssets{}, "asr", "vad"}, &fakeRuntime{}, nil, nil)
	defer e.Close()
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512)}) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.HistoryChanges():
	case <-time.After(time.Second):
		t.Fatal("live transcript missing")
	}
	before := History([]string{root})
	if len(before) != 1 || filepath.Ext(before[0].AudioPath) != ".wav" || before[0].TextPreview != "固定测试文本" {
		t.Fatalf("missing in-progress WAV transcript: %+v", before)
	}
	if len(before[0].CapturedRanges) == 0 {
		t.Fatal("live transcript was published before its capture timing was durable")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	after := History([]string{root})
	if len(after) != 1 || filepath.Ext(after[0].AudioPath) != ".flac" || after[0].TextPreview != before[0].TextPreview ||
		after[0].SessionID != before[0].SessionID || !reflect.DeepEqual(after[0].TranscriptChunks, before[0].TranscriptChunks) {
		t.Fatalf("FLAC finalization duplicated or lost a live transcript: %+v", after)
	}
	if len(after[0].CapturedRanges) == 0 || !reflect.DeepEqual(after[0].CapturedRanges, before[0].CapturedRanges) {
		t.Fatalf("FLAC finalization lost scene capture timing: %+v", after)
	}
}

func TestLateRecognitionStillStreamsAfterSegmentRotation(t *testing.T) {
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	defer func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()
	e, _, _, root := testEngine(t, false, &fakeRuntime{gate: gate, entered: entered})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512)}) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("recognition never started")
	}
	settings := e.Settings()
	settings.Recording.OutputDir = filepath.Join(root, "rotated")
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	close(gate)
	select {
	case <-e.HistoryChanges():
	case <-time.After(time.Second):
		t.Fatal("late recognition did not notify the timeline")
	}
	history := History([]string{root})
	if len(history) != 1 || history[0].TextPreview != "固定测试文本" || !e.Status().Recording {
		t.Fatalf("rotating hid late recognition or stopped recording: %+v", history)
	}
}
