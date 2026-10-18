package recorder

import (
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
)

func TestEchoCancellationToggleReopensCaptureAndPreservesRecording(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	settings := e.Settings()
	settings.Recording.EchoCancellation = true
	settings.Recording.AutoSwitchDevice = true
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if !e.Status().AutoSwitchEnabled {
		t.Fatal("echo cancellation must not silently disable the auto-switch preference")
	}
	capture.emit(make([]float32, audio.WindowSize))
	settings.Recording.EchoCancellation = false
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording || !e.Status().AutoSwitchEnabled {
		t.Fatal("changing processing did not retain recording and restore auto-switch preference")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 2 || !capture.options[0].EchoCancellation || capture.options[1].EchoCancellation {
		t.Fatalf("capture was not reopened with the requested processing: %+v", capture.options)
	}
}

func TestEchoCancellationRestartFailureStopsInsteadOfRecordingRaw(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	previous := e.Settings()
	settings := previous
	settings.Recording.Device = "unavailable"
	settings.Recording.EchoCancellation = !previous.Recording.EchoCancellation
	capture.mu.Lock()
	capture.failID = "unavailable"
	capture.mu.Unlock()
	if err := e.Apply(settings); err == nil {
		t.Fatal("capture failure was hidden")
	}
	if e.Status().Recording || e.Status().Error == nil || e.Settings() != previous {
		t.Fatal("failed processing restart left a misleading active recording or changed settings")
	}
}
