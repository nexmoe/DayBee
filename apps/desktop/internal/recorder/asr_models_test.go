package recorder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestModelChangeWhileRecordingAndIdle(t *testing.T) {
	runtime := &fakeRuntime{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	e, _, _, _ := testEngine(t, false, runtime)
	old := e.Settings()
	next := old
	next.Recording.ASRModel = protocol.ASRFireRedCTC
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(next); err != nil {
		t.Fatal("model switch during recording failed", err)
	}
	if e.Settings() != next || !e.Status().Recording {
		t.Fatal("model switch disrupted recording or lost the selection")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	if err := e.worker.SubmitBatch(asr.Job{Context: context.Background(), Prepare: func() ([]float32, int, error) { return []float32{.1}, 16000, nil }, Complete: func(asr.Result, error) { close(done) }}); err != nil {
		t.Fatal(err)
	}
	<-runtime.entered
	err := e.Apply(old)
	close(runtime.gate)
	if err == nil {
		t.Fatal("switched models during pending inference")
	}
	<-done
	deadline := time.Now().Add(time.Second)
	for e.worker.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := e.Apply(old); err != nil {
		t.Fatal("idle model switch failed", err)
	}
	if e.Settings().Recording.ASRModel != old.Recording.ASRModel {
		t.Fatal("idle selection was not applied")
	}
}

func TestSelectedModelRecordedForLiveAndBatchTranscription(t *testing.T) {
	e, _, _, root := testEngine(t, false, &fakeRuntime{})
	settings := e.Settings()
	settings.Recording.ASRModel = protocol.ASRFireRedCTC
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "selected.wav")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	seg := &segment{settings: settings.Recording, start: time.Now(), texts: []string{"live"}}
	if err := writeSegmentMetadata(seg, path); err != nil {
		t.Fatal(err)
	}
	assertModel := func() {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, "selected.json"))
		if err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["model"] != asrmodel.Name(protocol.ASRFireRedCTC) {
			t.Fatal("metadata identifies the wrong model", metadata["model"])
		}
	}
	assertModel()
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	assertModel()
}
