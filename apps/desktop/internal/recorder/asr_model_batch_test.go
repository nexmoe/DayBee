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

func TestLiveModelSwitchPreservesBatchSelection(t *testing.T) {
	for _, labeled := range []bool{false, true} {
		name := "whole file"
		if labeled {
			name = "voiceprint labeled"
		}
		t.Run(name, func(t *testing.T) {
			gate := make(chan struct{})
			var release sync.Once
			unblock := func() { release.Do(func() { close(gate) }) }
			runtime := &fakeRuntime{gate: gate, entered: make(chan struct{}, 1), modelText: true}
			e, _, _, root := testEngine(t, false, runtime)
			t.Cleanup(unblock)
			if labeled {
				installVoice(e, &scriptVoice{profile: acceptedLegacy()})
			}
			for _, name := range []string{"first.wav", "second.wav"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- e.Transcribe(root) }()
			select {
			case <-runtime.entered:
			case <-time.After(time.Second):
				t.Fatal("batch inference did not start")
			}
			settings := e.Settings()
			settings.Recording.ASRModel = protocol.ASRFireRedCTC
			if err := e.Apply(settings); err != nil {
				t.Fatal("live switch during batch failed", err)
			}
			if err := e.call(func() error {
				return e.consume(audio.Packet{Samples: make([]float32, audio.WindowSize)})
			}); err != nil {
				t.Fatal(err)
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("switch stranded batch work")
			}
			if err := e.Stop(); err != nil {
				t.Fatal(err)
			}
			files, err := AudioFiles([]string{root})
			if err != nil || len(files) != 3 {
				t.Fatal("missing batch or live audio", files, err)
			}
			for _, path := range files {
				model := protocol.DefaultASRModel
				if strings.HasPrefix(filepath.Base(path), "daybee_") {
					model = protocol.ASRFireRedCTC
				}
				raw, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
				if err != nil {
					t.Fatal(err)
				}
				var metadata struct{ Model, Text string }
				if err := json.Unmarshal(raw, &metadata); err != nil {
					t.Fatal(err)
				}
				if metadata.Model != asrmodel.Name(model) || metadata.Text != string(model) {
					t.Fatalf("batch and live models were mixed: %s", raw)
				}
			}
		})
	}
}
