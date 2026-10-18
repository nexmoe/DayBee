package recorder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestSpanFreezesProfileAndDropsMismatchedBadge(t *testing.T) {
	e, root := spanEngine(t, &countRuntime{})
	voice := &scriptVoice{profile: calibratedProfile("mic-1", false), splits: true, hold: make(chan struct{}), release: make(chan struct{})}
	installVoice(e, voice)
	voice.mu.Lock()
	started := voice.hold
	voice.mu.Unlock()
	e.mu.Lock()
	e.activeDevice = "mic-2"
	e.mu.Unlock()
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		return e.appendSpeech(&e.audioStream, asr.Speech{Samples: make([]float32, 48000)})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("match did not start")
	}
	voice.mu.Lock()
	voice.profile = nil
	voice.mu.Unlock()
	close(voice.release)
	stopSoon(t, e)
	voice.mu.Lock()
	seen := voice.seen
	voice.mu.Unlock()
	if seen == nil || seen.CreatedAt != "frozen" || seen.Device != "mic-1" {
		t.Fatalf("snapshot changed: %+v", seen)
	}
	items := History([]string{root})
	if len(items) != 1 || len(items[0].TranscriptChunks) != 1 || items[0].TextPreview != "段" {
		t.Fatalf("text dropped: %+v", items)
	}
	for _, chunk := range items[0].TranscriptChunks {
		if chunk.VoiceIdentity != "" {
			t.Fatalf("mismatch kept a badge: %+v", items[0].TranscriptChunks)
		}
	}
}

type cancelRuntime struct {
	fakeRuntime
	cancel func()
	n      atomic.Int32
}

func (r *cancelRuntime) NewRecognizer(string) (asr.Recognizer, error) {
	return &cancelRecognizer{runtime: r}, nil
}

type cancelRecognizer struct{ runtime *cancelRuntime }

func (r *cancelRecognizer) Decode([]float32, int) (asr.Result, error) {
	if r.runtime.n.Add(1) == 1 {
		r.runtime.cancel()
	}
	return asr.Result{Text: "段", Language: "zh"}, nil
}
func (r *cancelRecognizer) Close() {}

func TestBatchCancelAfterLastDecodeWritesNoTranscript(t *testing.T) {
	dir := t.TempDir()
	settings := protocol.DefaultSettings(dir)
	settings.Recording.AudioFormat = protocol.WAV
	settings.Recording.AutoSwitchDevice = false
	assets := &fakeAssets{}
	rt := &cancelRuntime{}
	engine := New(settings, &fakeCapture{}, assets, rt, nil, nil)
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	rt.cancel = engine.CancelBatch
	installVoice(engine, &scriptVoice{profile: acceptedLegacy(), splits: true})
	start := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	samples := make([]float32, 48000)
	writeMic(t, dir, "daybee_20261012_010000_microphone.wav", samples, start, map[string]any{
		"capture_device_id": "mic-1", "echo_cancellation": false,
	})
	assets.read = func(context.Context, string) ([]float32, int, error) { return samples, audio.SampleRate, nil }
	if err := engine.Transcribe(dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	transcript := filepath.Join(dir, "daybee_20261012_010000_microphone.json")
	if _, err := os.Stat(transcript); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial transcript metadata was written", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "daybee_20261012_010000_microphone.wav"))
	if err != nil || string(raw) != "mic" {
		t.Fatalf("original audio changed: %v %q", err, raw)
	}
}
