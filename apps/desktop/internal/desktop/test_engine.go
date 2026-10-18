package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

// This deterministic engine is selected only by DAYBEE_E2E_TEST, with an isolated
// store. Production recordings always use the same native recorder.Engine.
type testEngine struct {
	mu             sync.Mutex
	settings       protocol.AppSettings
	status         protocol.RecorderStatusSnapshot
	changed        func()
	started        time.Time
	resume         bool
	historyChanged chan struct{}
	transcripts    []string
}

func newTestEngine(settings protocol.AppSettings, changed func()) *testEngine {
	e := &testEngine{settings: settings, status: protocol.DefaultStatus(), changed: changed, historyChanged: make(chan struct{}, 1)}
	e.status.DeviceLabel = "MacBook Pro Microphone"
	e.status.SenseVoiceReady = true
	e.status.VADReady = true
	e.status.StatusMessage = "E2E desktop engine ready."
	_ = e.Apply(settings)
	return e
}
func (e *testEngine) Devices() []protocol.DeviceInfo {
	return []protocol.DeviceInfo{{ID: "default", Index: 0, IsDefault: true, Label: "MacBook Pro Microphone"}, {ID: "usb-mic", Index: 1, Label: "USB Podcast Mic"}}
}
func (e *testEngine) HistoryChanges() <-chan struct{} { return e.historyChanged }
func (e *testEngine) Status() protocol.RecorderStatusSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.ASRHistory = append([]string{}, s.ASRHistory...)
	s.WaveformBins = append([]float64{}, s.WaveformBins...)
	return s
}
func (e *testEngine) Apply(settings protocol.AppSettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settings = settings
	e.status.ASREnabled = !settings.Recording.DisableASR
	e.status.AutoSwitchEnabled = settings.Recording.AutoSwitchDevice
	e.status.DeviceLabel = "MacBook Pro Microphone"
	if settings.Recording.Device == "usb-mic" {
		e.status.DeviceLabel = "USB Podcast Mic"
	}
	e.changed()
	return nil
}
func (e *testEngine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status.Recording {
		return nil
	}
	e.started = time.Now()
	e.transcripts = nil
	e.status.Recording = true
	e.status.InSpeech = true
	e.status.Error = nil
	e.status.ASRHistory = []string{}
	if !e.settings.Recording.DisableASR {
		e.status.ASRPreview = "Listening for speech in test mode."
	}
	e.status.StatusMessage = "Test recording in progress."
	e.changed()
	return nil
}
func (e *testEngine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.Recording = false
	e.status.InSpeech = false
	e.status.ASRPreview = ""
	e.status.Elapsed = "00:00:00"
	if !e.settings.Recording.DisableASR {
		e.status.ASRHistory = []string{"Playwright test transcript."}
	}
	e.status.StatusMessage = "Test recording stopped."
	e.changed()
	return nil
}
func (e *testEngine) Transcribe(_ string) error {
	e.mu.Lock()
	e.status.StatusMessage = "Test transcription finished."
	e.mu.Unlock()
	e.changed()
	return nil
}

// The private native-test endpoint simulates completed inference, not capture.
func (e *testEngine) completeSpeech(text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.status.Recording || !e.status.ASREnabled {
		return errors.New("Test speech requires recording with transcription enabled")
	}
	e.transcripts = append(e.transcripts, text)
	path := filepath.Join(e.settings.Recording.OutputDir, e.started.Format("20060102"), fmt.Sprintf("e2e_live_%d.wav", e.started.UnixNano()))
	if err := config.WriteJSON(strings.TrimSuffix(path, ".wav")+".json", map[string]any{
		"audio_path": path, "segment_start_time": e.started.Format(time.RFC3339), "status": "ok", "text": strings.Join(e.transcripts, "\n"),
		"recording_session_id": e.started.Format(time.RFC3339Nano),
	}); err != nil {
		return err
	}
	e.status.ASRPreview = text
	select {
	case e.historyChanged <- struct{}{}:
	default:
	}
	e.changed()
	return nil
}
func (e *testEngine) Busy() bool     { return e.Status().Recording }
func (e *testEngine) Suspend() error { e.resume = e.Busy(); return e.Stop() }
func (e *testEngine) Resume() error {
	if e.resume {
		e.resume = false
		return e.Start()
	}
	return nil
}
func (e *testEngine) Close() error { return e.Stop() }
