package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestPureRecordingFlushesVADWithoutASR(t *testing.T) {
	runtime := &fakeRuntime{hold: true}
	e, capture, assets, root := testEngine(t, true, runtime)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if e.Status().StatusMessage != "Recording audio only." {
		t.Fatal("pure recording message changed")
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	capture.emit(make([]float32, 600))
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	files, err := AudioFiles([]string{root})
	if err != nil || len(files) != 1 {
		t.Fatal("speech was not flushed", files, err)
	}
	stat, _ := os.Stat(files[0])
	if stat.Size() != 44+1024*2 {
		t.Fatal("tail not flushed", stat.Size())
	}
	if _, err = os.Stat(strings.TrimSuffix(files[0], ".wav") + ".json"); !os.IsNotExist(err) {
		t.Fatal("pure recording wrote transcript JSON")
	}
	if assets.asrRequested || runtime.recognizers != 0 {
		t.Fatal("ASR model used in pure recording")
	}
	if runtime.vadsClosed != 1 {
		t.Fatal("VAD not freed")
	}
}
func TestOverloadArchivesAllSpeechAndDefersStop(t *testing.T) {
	runtime := &fakeRuntime{gate: make(chan struct{})}
	e, _, _, root := testEngine(t, false, runtime)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512), DeviceLabel: "Test Mic"}) }); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- e.Stop() }()
	select {
	case <-done:
		t.Fatal("stop did not wait for inference")
	case <-time.After(30 * time.Millisecond):
	}
	close(runtime.gate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop stalled")
	}
	files, _ := AudioFiles([]string{root})
	if len(files) != 1 {
		t.Fatal(files)
	}
	stat, _ := os.Stat(files[0])
	if stat.Size() != 44+20*512*2 {
		t.Fatal("overload lost archived audio")
	}
	raw, _ := os.ReadFile(strings.TrimSuffix(files[0], ".wav") + ".json")
	var metadata map[string]any
	_ = json.Unmarshal(raw, &metadata)
	if metadata["transcription_incomplete"] != true || metadata["status"] != "ok" {
		t.Fatal("missing internal overload diagnostics", string(raw))
	}
	if e.Busy() {
		t.Fatal("engine remained busy")
	}
}
func TestSettingsRotateUsingPreviousConfiguration(t *testing.T) {
	runtime := &fakeRuntime{}
	e, _, _, root := testEngine(t, false, runtime)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if e.Status().StatusMessage != "Recording with Whisper Tiny." {
		t.Fatal("live recording message changed")
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.ASRPreview = "Previous segment"
		s.ASRHistory = []string{"Previous speech"}
	})
	feed := func() {
		t.Helper()
		if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, 512), DeviceLabel: "Test Mic"}) }); err != nil {
			t.Fatal(err)
		}
	}
	feed()
	settings := e.Settings()
	settings.Recording.DisableASR = true
	settings.Recording.OutputDir = filepath.Join(root, "new")
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if status := e.Status(); status.ASRPreview != "" || len(status.ASRHistory) != 0 || status.StatusMessage != "Started a new recording segment." {
		t.Fatal("ASR disable did not preserve the old cleared-preview behavior", status)
	}
	feed()
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	oldFiles, _ := AudioFiles([]string{root})
	if len(oldFiles) != 2 {
		t.Fatal("rotation lost a segment", oldFiles)
	}
	jsonCount := 0
	for _, path := range oldFiles {
		_, err := os.Stat(strings.TrimSuffix(path, ".wav") + ".json")
		if err == nil {
			jsonCount++
			if strings.Contains(path, filepath.Join(root, "new")) {
				t.Fatal("new ASR-disabled segment has JSON")
			}
		}
	}
	if jsonCount != 1 {
		t.Fatal("old segment did not retain ASR configuration")
	}
}
func TestFLACFailureRetainsPlayableWAV(t *testing.T) {
	e, _, _, root := testEngine(t, false, &fakeRuntime{})
	settings := e.Settings()
	settings.Recording.AudioFormat = protocol.FLAC
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
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
	files, _ := AudioFiles([]string{root})
	if len(files) != 1 || filepath.Ext(files[0]) != ".wav" {
		t.Fatal("WAV was not preserved", files)
	}
}
func TestSuspendResumesOnlyPreviousRecording(t *testing.T) {
	e, _, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	if err := e.Resume(); err != nil || e.Status().Recording {
		t.Fatal("idle resumed", err)
	}
	_ = e.Start()
	_ = e.Suspend()
	if e.Status().Recording {
		t.Fatal("sleep did not stop")
	}
	_ = e.Resume()
	if !e.Status().Recording {
		t.Fatal("recording did not resume")
	}
	_ = e.Suspend()
	_ = e.Stop()
	_ = e.Resume()
	if e.Status().Recording {
		t.Fatal("manual stop was ignored")
	}
}
