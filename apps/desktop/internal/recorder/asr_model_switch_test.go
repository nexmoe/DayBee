package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestLiveModelSwitchKeepsCaptureAndQueuedSpeech(t *testing.T) {
	for _, systemAudio := range []bool{false, true} {
		name := "microphone"
		if systemAudio {
			name = "microphone and system audio"
		}
		t.Run(name, func(t *testing.T) {
			gate := make(chan struct{})
			var release sync.Once
			unblock := func() { release.Do(func() { close(gate) }) }
			runtime := &fakeRuntime{gate: gate, entered: make(chan struct{}, 1), modelText: true}
			e, capture, _, root := testEngine(t, false, runtime)
			t.Cleanup(unblock)
			settings := e.Settings()
			settings.Recording.RecordSystemAudio = systemAudio
			if err := e.Apply(settings); err != nil {
				t.Fatal(err)
			}
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			started := e.started
			sources := []protocol.AudioSource{protocol.SourceMicrophone}
			if systemAudio {
				sources = append(sources, protocol.SourceSystem)
			}
			feed := func(n int) {
				t.Helper()
				for _, source := range sources {
					if err := e.call(func() error {
						return e.consume(audio.Packet{Source: source, Samples: make([]float32, n), DeviceLabel: "Test Mic"})
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
			feed(audio.WindowSize)
			select {
			case <-runtime.entered:
			case <-time.After(time.Second):
				t.Fatal("first decode did not start")
			}
			// Leave both queued speech and a partial VAD window at the boundary.
			feed(audio.WindowSize)
			feed(200)
			settings.Recording.ASRModel = protocol.ASRFireRedCTC
			switched := make(chan error, 1)
			go func() { switched <- e.Apply(settings) }()
			select {
			case err := <-switched:
				if err != nil {
					t.Fatal("live switch failed", err)
				}
			case <-time.After(time.Second):
				t.Fatal("model switch waited for inference")
			}
			capture.mu.Lock()
			starts, capturing := len(capture.options), capture.callback != nil
			capture.mu.Unlock()
			if !e.Status().Recording || e.started != started || starts != 1 || !capturing {
				t.Fatal("model switch restarted or stopped capture")
			}
			// Drain the old queue before submitting new speech to avoid overload.
			unblock()
			e.finalizers.Wait()
			feed(audio.WindowSize)
			if err := e.Stop(); err != nil {
				t.Fatal(err)
			}
			files, err := AudioFiles([]string{root})
			if err != nil || len(files) != len(sources)*2 {
				t.Fatal("model switch lost an archive", files, err)
			}
			counts := map[string]int{}
			for _, path := range files {
				raw, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
				if err != nil {
					t.Fatal(err)
				}
				var metadata struct {
					Model   string `json:"model"`
					Text    string `json:"text"`
					Session string `json:"recording_session_id"`
				}
				if err := json.Unmarshal(raw, &metadata); err != nil {
					t.Fatal(err)
				}
				model, windows := protocol.DefaultASRModel, 3
				if metadata.Model == asrmodel.Name(protocol.ASRFireRedCTC) {
					model, windows = protocol.ASRFireRedCTC, 1
				}
				want := strings.TrimSuffix(strings.Repeat(string(model)+"\n", windows), "\n")
				if metadata.Model != asrmodel.Name(model) || metadata.Text != want || metadata.Session != started.Format(time.RFC3339Nano) {
					t.Fatalf("speech used the wrong model or session: %s", raw)
				}
				stat, err := os.Stat(path)
				if err != nil || stat.Size() != int64(44+windows*audio.WindowSize*2) {
					t.Fatal("switch lost archived samples", path, err)
				}
				counts[metadata.Model]++
			}
			if counts[asrmodel.Name(protocol.DefaultASRModel)] != len(sources) || counts[asrmodel.Name(protocol.ASRFireRedCTC)] != len(sources) {
				t.Fatal("archives did not retain both models", counts)
			}
		})
	}
}
