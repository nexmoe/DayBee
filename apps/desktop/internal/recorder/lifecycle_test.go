package recorder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
)

func TestFailedDeviceChangeRollsBackAndFinalizes(t *testing.T) {
	e, capture, _, root := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	before := e.Settings()
	capture.emit(make([]float32, 512))
	capture.mu.Lock()
	capture.failID = "missing"
	capture.mu.Unlock()
	next := before
	next.Recording.Device = "missing"
	if err := e.Apply(next); err == nil {
		t.Fatal("unavailable device accepted")
	}
	if e.Settings() != before || e.Status().Recording || e.Busy() {
		t.Fatal("failed switch left inconsistent engine state")
	}
	files, _ := AudioFiles([]string{root})
	if len(files) != 1 {
		t.Fatal("previous audio was not finalized", files)
	}
	if err := e.Start(); err != nil {
		t.Fatal("could not restart previous device", err)
	}
}

func TestPCMOverflowStopsAndDrainsAcceptedSamples(t *testing.T) {
	e, capture, _, root := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	blocked := make(chan error, 1)
	go func() { blocked <- e.call(func() error { close(entered); <-release; return nil }) }()
	<-entered
	for range 80 {
		capture.emit(make([]float32, 512))
	}
	close(release)
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for e.Status().Recording && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := e.Status()
	if status.Recording || status.Error == nil || *status.Error != ErrPCMOverflow.Error() {
		t.Fatal("PCM overflow did not stop safely", status)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	files, _ := AudioFiles([]string{root})
	if len(files) != 1 {
		t.Fatal(files)
	}
	stat, _ := os.Stat(files[0])
	if stat.Size() != 44+64*512*2 {
		t.Fatal("accepted PCM was lost", stat.Size())
	}
}

func TestCloseWaitsForActiveBatchAndCancelsRemainingFiles(t *testing.T) {
	runtime := &fakeRuntime{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	e, _, _, root := testEngine(t, true, runtime)
	for _, name := range []string{"a.wav", "b.wav"} {
		writer, err := audio.NewWavWriter(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = writer.Append([]float32{.1, .2}); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	transcribed := make(chan error, 1)
	go func() { transcribed <- e.Transcribe(root) }()
	select {
	case <-runtime.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("batch did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- e.Close() }()
	select {
	case <-closed:
		t.Fatal("native decode was not awaited")
	case <-time.After(20 * time.Millisecond):
	}
	close(runtime.gate)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("batch shutdown deadlocked")
	}
	if err := <-transcribed; !errors.Is(err, context.Canceled) {
		t.Fatal("queued batch was not canceled", err)
	}
	for _, name := range []string{"a.wav", "b.wav"} {
		if _, err := os.Stat(filepath.Join(root, strings.TrimSuffix(name, ".wav")+".json")); !os.IsNotExist(err) {
			t.Fatal("canceled batch wrote JSON")
		}
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.recognizers != 1 || runtime.releases != 1 {
		t.Fatal("ASR native resources leaked")
	}
}
