package audio

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

// Opt-in: this opens both sources and plays only the provided speech fixture.
// Ordinary tests never request screen/audio capture authorization.
func TestSystemAudioHardwareCapture(t *testing.T) {
	wav := os.Getenv("DAYBEE_TEST_SYSTEM_AUDIO_WAV")
	if wav == "" {
		t.Skip("Set DAYBEE_TEST_SYSTEM_AUDIO_WAV to verify native system capture with known playback")
	}
	requireRecorderStopped(t)
	capture, err := NewNativeCapture()
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	var mu sync.Mutex
	var system, microphoneSamples []float32
	var microphone atomic.Int64
	var stopped, collecting atomic.Bool
	var late atomic.Int64
	failures := make(chan error, 2)
	if err := capture.Start(CaptureOptions{DeviceID: "default", EchoCancellation: true, SystemAudio: true}, func(packet Packet) {
		if stopped.Load() {
			late.Add(1)
		}
		if !collecting.Load() {
			return
		}
		if packet.Source == protocol.SourceSystem {
			mu.Lock()
			system = append(system, packet.Samples...)
			mu.Unlock()
		} else if packet.Source == protocol.SourceMicrophone {
			microphone.Add(int64(len(packet.Samples)))
			mu.Lock()
			microphoneSamples = append(microphoneSamples, packet.Samples...)
			mu.Unlock()
		} else {
			t.Errorf("unknown native audio source: %s", packet.Source)
		}
	}, func(err error) {
		select {
		case failures <- err:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	collecting.Store(true)
	if output, err := exec.Command("afplay", wav).CombinedOutput(); err != nil {
		t.Fatalf("play fixture: %s %v", output, err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	stopped.Store(true)
	time.Sleep(50 * time.Millisecond)
	requireRecorderStopped(t)
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	mu.Lock()
	defer mu.Unlock()
	if len(system) < SampleRate || microphone.Load() < SampleRate || RMS(system) < .001 || late.Load() != 0 {
		t.Fatalf("invalid dual source capture: system=%d rms=%f microphone=%d late=%d", len(system), RMS(system), microphone.Load(), late.Load())
	}
	for output, samples := range map[string][]float32{
		os.Getenv("DAYBEE_TEST_SYSTEM_CAPTURE_WAV"): system,
		os.Getenv("DAYBEE_TEST_MIC_CAPTURE_WAV"):    microphoneSamples,
	} {
		if output == "" {
			continue
		}
		writer, err := NewWavWriter(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := writer.Append(samples); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("Captured %d system and %d microphone samples with distinct source labels", len(system), microphone.Load())
}
