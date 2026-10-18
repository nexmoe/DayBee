package recorder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

type systemTestAssets struct {
	*fakeAssets
	key          atomic.Value
	failLanguage string
}

func (a *systemTestAssets) SelectASRModel(id protocol.ASRModel, language ...string) {
	a.fakeAssets.SelectASRModel(id)
	a.key.Store(a.ASRModelPath(id, language...))
}
func (a *systemTestAssets) ASRPath() string { return a.key.Load().(string) }
func (a *systemTestAssets) ASRModelPath(id protocol.ASRModel, language ...string) string {
	if id == protocol.ASRSpeechAnalyzer && len(language) > 0 {
		return speechanalyzer.ModelKey(language[0])
	}
	return string(id)
}
func (a *systemTestAssets) EnsureASRModel(ctx context.Context, _ protocol.ASRModel, language ...string) error {
	if len(language) > 0 && language[0] == a.failLanguage {
		return errors.New("System model preparation failed")
	}
	return ctx.Err()
}

func systemTestEngine(t *testing.T, runtime *fakeRuntime, failLanguage string) (*Engine, *fakeCapture, *systemTestAssets, string) {
	t.Helper()
	root := t.TempDir()
	settings := protocol.DefaultSettings(root)
	settings.Recording.ASRModel = protocol.ASRSpeechAnalyzer
	settings.Recording.ASRLanguage = "en"
	settings.Recording.AutoSwitchDevice = false
	capture, assets := &fakeCapture{}, &systemTestAssets{fakeAssets: &fakeAssets{}, failLanguage: failLanguage}
	e := New(settings, capture, assets, runtime, nil, nil)
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e, capture, assets, root
}

func TestSpeechLanguageSwitchKeepsQueuedLanguageAndMetadata(t *testing.T) {
	gate := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(gate) }) }
	runtime := &fakeRuntime{gate: gate, entered: make(chan struct{}, 1), modelText: true}
	e, capture, _, root := systemTestEngine(t, runtime, "")
	t.Cleanup(unblock)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	feed := func() {
		t.Helper()
		if err := e.call(func() error { return e.consume(audio.Packet{Samples: make([]float32, audio.WindowSize)}) }); err != nil {
			t.Fatal(err)
		}
	}
	feed()
	select {
	case <-runtime.entered:
	case <-time.After(time.Second):
		t.Fatal("first decode did not start")
	}
	feed() // This queued utterance must retain English after the switch.
	settings := e.Settings()
	settings.Recording.ASRLanguage = "zh"
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	starts := len(capture.options)
	capture.mu.Unlock()
	if !e.Status().Recording || starts != 1 {
		t.Fatal("language switch restarted capture")
	}
	feed()
	unblock()
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	files, err := AudioFiles([]string{root})
	if err != nil || len(files) != 2 {
		t.Fatal("language switch lost an archive", files, err)
	}
	texts := map[string]bool{}
	for _, path := range files {
		raw, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var metadata struct{ Backend, Model, Text string }
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Backend != "speech-analyzer" || metadata.Model != "SpeechAnalyzer" {
			t.Fatal("wrong native model metadata", string(raw))
		}
		texts[metadata.Text] = true
	}
	if !texts["speech-analyzer:en\nspeech-analyzer:en"] || !texts["speech-analyzer:zh"] {
		t.Fatal("queued speech changed language", texts)
	}
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var metadata struct{ Backend, Text string }
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Backend != "speech-analyzer" || metadata.Text != "speech-analyzer:zh" {
			t.Fatal("batch did not retain its native model and language", string(raw))
		}
	}
}

func TestSpeechLanguagePreparationFailurePreservesRecordingAndSelection(t *testing.T) {
	e, _, assets, _ := systemTestEngine(t, &fakeRuntime{}, "zh")
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	settings := e.Settings()
	settings.Recording.ASRLanguage = "zh"
	if err := e.Apply(settings); err == nil {
		t.Fatal("failed system preparation accepted")
	}
	if !e.Status().Recording || e.Settings().Recording.ASRLanguage != "en" || assets.ASRPath() != speechanalyzer.ModelKey("en") {
		t.Fatal("failed system preparation interrupted recording or changed selection")
	}
}

func TestSpeechAutoLanguagePreservesSystemSelectionAcrossUIChanges(t *testing.T) {
	e, _, assets, _ := systemTestEngine(t, &fakeRuntime{}, "")
	settings := e.Settings()
	settings.Recording.ASRLanguage = "auto"
	settings.Desktop.Language = protocol.LanguageChinese
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if assets.ASRPath() != speechanalyzer.ModelKey("auto") {
		t.Fatal("UI language overrode the system transcription language", assets.ASRPath())
	}
	// UI language changes must leave system-language transcription selected.
	settings.Desktop.Language = protocol.LanguageEnglish
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if assets.ASRPath() != speechanalyzer.ModelKey("auto") {
		t.Fatal("system transcription language changed with the UI", assets.ASRPath())
	}
	settings.Recording.ASRLanguage = "ja"
	settings.Desktop.Language = protocol.LanguageChinese
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if assets.ASRPath() != speechanalyzer.ModelKey("ja") {
		t.Fatal("app language overrode an explicit speech-language choice", assets.ASRPath())
	}
}
