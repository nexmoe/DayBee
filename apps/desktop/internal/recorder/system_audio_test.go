package recorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func enableSystemAudio(t *testing.T, e *Engine) {
	t.Helper()
	s := e.Settings()
	s.Recording.RecordSystemAudio = true
	if err := e.Apply(s); err != nil {
		t.Fatal(err)
	}
}

func sourcePacket(source protocol.AudioSource, count int, value float32) audio.Packet {
	samples := make([]float32, count)
	for i := range samples {
		samples[i] = value
	}
	return audio.Packet{Source: source, Samples: samples, DeviceID: string(source), DeviceLabel: string(source)}
}

func TestSystemAudioArchivesAndTranscribesEachSourceIndependently(t *testing.T) {
	runtime := &fakeRuntime{}
	e, _, _, root := testEngine(t, false, runtime)
	enableSystemAudio(t, e)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	// Interleaving partial VAD windows must never combine microphone/system PCM.
	for _, packet := range []audio.Packet{
		sourcePacket(protocol.SourceMicrophone, 320, .1), sourcePacket(protocol.SourceSystem, 384, .2),
		sourcePacket(protocol.SourceMicrophone, 192, .1), sourcePacket(protocol.SourceSystem, 128, .2),
	} {
		if err := e.call(func() error { return e.consume(packet) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root})
	if len(items) != 2 {
		t.Fatalf("expected two source archives: %+v", items)
	}
	seen := map[protocol.AudioSource]bool{}
	for _, item := range items {
		seen[item.AudioSource] = true
		if !strings.Contains(item.AudioPath, "_"+string(item.AudioSource)) || len(item.TranscriptChunks) != 1 || item.TextPreview != "固定测试文本" {
			t.Fatalf("source transcript missing: %+v", item)
		}
		samples := archivedSamples(t, item.AudioPath)
		want := float32(.1)
		if item.AudioSource == protocol.SourceSystem {
			want = .2
		}
		if len(samples) != 512 {
			t.Fatalf("mixed source windows: %d samples", len(samples))
		}
		for _, sample := range samples {
			if sample != float32(int16(float64(want)*32767))/32768 {
				t.Fatalf("source PCM mixed: %s got %v", item.AudioSource, sample)
			}
		}
		for _, suffix := range []string{".json", ".recording.json"} {
			raw, err := os.ReadFile(strings.TrimSuffix(item.AudioPath, filepath.Ext(item.AudioPath)) + suffix)
			if err != nil || !strings.Contains(string(raw), `"audio_source": "`+string(item.AudioSource)+`"`) {
				t.Fatalf("source absent from durable %s: %s %v", suffix, raw, err)
			}
		}
	}
	if !seen[protocol.SourceMicrophone] || !seen[protocol.SourceSystem] {
		t.Fatal("source identity lost")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.vadsClosed != 2 {
		t.Fatalf("independent VADs were not closed: %d", runtime.vadsClosed)
	}
}

func TestSystemAudioToggleClosesSourceAndIgnoresLatePackets(t *testing.T) {
	e, capture, _, root := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	enableSystemAudio(t, e)
	if err := e.call(func() error { return e.consume(sourcePacket(protocol.SourceSystem, 512, .2)) }); err != nil {
		t.Fatal(err)
	}
	s := e.Settings()
	s.Recording.RecordSystemAudio = false
	if err := e.Apply(s); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording {
		t.Fatal("toggle stopped microphone recording")
	}
	if err := e.call(func() error {
		if err := e.consume(sourcePacket(protocol.SourceSystem, 512, .8)); err != nil {
			return err
		}
		return e.consume(sourcePacket(protocol.SourceMicrophone, 512, .1))
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if items := History([]string{root}); len(items) != 2 {
		t.Fatalf("late system packet created an archive: %+v", items)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 3 || capture.options[0].SystemAudio || !capture.options[1].SystemAudio || capture.options[2].SystemAudio {
		t.Fatalf("native sources did not follow toggle: %+v", capture.options)
	}
}

func TestSystemAudioSourceSurvivesAudioOnlyAndBatchReprocessing(t *testing.T) {
	e, _, _, root := testEngine(t, true, &fakeRuntime{})
	enableSystemAudio(t, e)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error { return e.consume(sourcePacket(protocol.SourceSystem, 512, .2)) }); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.Transcribe(root); err != nil {
			t.Fatal(err)
		}
		items := History([]string{root})
		if len(items) != 1 || items[0].AudioSource != protocol.SourceSystem || items[0].TextPreview == "" || len(items[0].CapturedRanges) != 1 {
			t.Fatalf("reprocessing lost source/timing: %+v", items)
		}
	}
}

func TestSystemAudioPermissionFailureRollsBackAndStopsBothSources(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	previous := e.Settings()
	capture.mu.Lock()
	capture.failSystem = true
	capture.mu.Unlock()
	s := previous
	s.Recording.RecordSystemAudio = true
	if err := e.Apply(s); err == nil {
		t.Fatal("system permission failure was hidden")
	}
	if e.Status().Recording || e.Status().Error == nil || e.Settings() != previous {
		t.Fatal("failed system capture left misleading active sources or changed settings")
	}
}
