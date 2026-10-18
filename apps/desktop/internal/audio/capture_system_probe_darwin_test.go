package audio

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in, in-memory playback reference only. Does not open the microphone,
// alter the running recorder, play a sound or save any captured samples.
func TestSystemReferenceHardwareProbe(t *testing.T) {
	if os.Getenv("DAYBEE_TEST_REFERENCE_PROBE") != "1" {
		t.Skip("Set DAYBEE_TEST_REFERENCE_PROBE=1 to check native playback capture")
	}
	var samples atomic.Int64
	failures := make(chan error, 1)
	capture, err := newSystemCapture(func(packet []float32) {
		samples.Add(int64(len(packet)))
	}, func(err error) {
		select {
		case failures <- err:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop()
	if err := capture.Start(); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case err := <-failures:
		t.Fatalf("native playback reference stopped after %d samples: %v", samples.Load(), err)
	case <-timer.C:
	}
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	if samples.Load() < SampleRate {
		t.Fatalf("native playback reference delivered only %d samples in 8 seconds", samples.Load())
	}
	t.Logf("Native playback reference stayed active for 8 seconds: %d samples; no microphone or audio files", samples.Load())
}
