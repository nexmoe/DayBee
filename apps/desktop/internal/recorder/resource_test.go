package recorder

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestStopReleasesRecognizerAndRecordingCanRestart(t *testing.T) {
	runtime := &fakeRuntime{entered: make(chan struct{}, 2)}
	e, capture, _, _ := testEngine(t, false, runtime)
	for take := 1; take <= 2; take++ {
		if err := e.Start(); err != nil {
			t.Fatal(err)
		}
		capture.emit(make([]float32, 512))
		select {
		case <-runtime.entered:
		case <-time.After(time.Second):
			t.Fatal("speech did not reach the recognizer")
		}
		if err := e.Stop(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			runtime.mu.Lock()
			created, released := runtime.recognizers, runtime.releases
			runtime.mu.Unlock()
			if released == take {
				if created != take {
					t.Fatalf("restart did not reload the model: created=%d take=%d", created, take)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("stopped recording retained the native model: created=%d released=%d", created, released)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWaveformCanPauseWithoutPausingAudioArchive(t *testing.T) {
	e, capture, _, root := testEngine(t, true, &fakeRuntime{})
	frames := make(chan struct{}, 8)
	var count atomic.Int64
	if err := e.call(func() error {
		e.waveform = func(protocol.WaveformFrame) { count.Add(1); frames <- struct{}{} }
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	e.SetWaveformEnabled(false)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	capture.emit(make([]float32, audio.WindowSize))
	time.Sleep(100 * time.Millisecond)
	if count.Load() != 0 {
		t.Fatal("hidden waveform still computed and published frames")
	}
	e.SetWaveformEnabled(true)
	select {
	case <-frames:
	case <-time.After(time.Second):
		t.Fatal("waveform did not resume when shown")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	files, err := AudioFiles([]string{root})
	if err != nil || len(files) != 1 {
		t.Fatal("pausing visualization prevented audio archiving", files, err)
	}
}
