package audio

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVoiceBufferKeepsSpeechOrderedAndReportsOverflow(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "voice-buffer-test")
	command := exec.Command("xcrun", "clang", "-std=c11", "-pthread", "-fsanitize=address,undefined", "testdata/voice_buffer_test.c", "-o", binary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile buffer stress test: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native audio buffer lost or overwrote speech: %v\n%s", err, output)
	}
}

// Opt-in only: normal tests never open the microphone or play sound.
func TestVoiceCaptureHardwareLifecycle(t *testing.T) {
	if os.Getenv("DAYBEE_TEST_MICROPHONE") != "1" {
		t.Skip("Set DAYBEE_TEST_MICROPHONE=1 for microphone lifecycle verification")
	}
	capture, err := NewNativeCapture()
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	devices, err := capture.Devices()
	if err != nil || len(devices) == 0 {
		t.Fatal("no microphone", err)
	}
	id := "default"
	for _, device := range devices {
		if device.IsDefault {
			id = device.ID
		}
	}
	for _, device := range []string{"default", id} {
		var samples, late atomic.Int64
		var stopped atomic.Bool
		failures := make(chan error, 1)
		if err := capture.Start(CaptureOptions{DeviceID: device, EchoCancellation: true}, func(packet Packet) {
			if stopped.Load() {
				late.Add(1)
			}
			samples.Add(int64(len(packet.Samples)))
		}, func(err error) {
			select {
			case failures <- err:
			default:
			}
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		if err := capture.Stop(); err != nil {
			t.Fatal(err)
		}
		stopped.Store(true)
		time.Sleep(50 * time.Millisecond)
		select {
		case err := <-failures:
			t.Fatal(err)
		default:
		}
		if samples.Load() < SampleRate || samples.Load() > 3*SampleRate || late.Load() != 0 {
			t.Fatalf("invalid capture lifecycle: samples=%d late=%d", samples.Load(), late.Load())
		}
	}
}

func TestVoiceCaptureSpeakerComparison(t *testing.T) {
	wav := os.Getenv("DAYBEE_TEST_SPEAKER_WAV")
	if wav == "" {
		t.Skip("Set DAYBEE_TEST_SPEAKER_WAV to a speech WAV, with built-in speakers and microphone selected")
	}
	var captures [2]*NativeCapture
	var recordings [2][]float32
	var locks [2]sync.Mutex
	var collecting atomic.Bool
	failures := make(chan error, 2)
	// Capture both paths during the same playback, after voice processing has
	// settled. Sequential runs can measure different device gains or ducking.
	for _, index := range []int{1, 0} {
		capture, err := NewNativeCapture()
		if err != nil {
			t.Fatal(err)
		}
		captures[index] = capture
		t.Cleanup(func() { capture.Close() })
		if err := capture.Start(CaptureOptions{DeviceID: "default", EchoCancellation: index == 1}, func(packet Packet) {
			if !collecting.Load() {
				return
			}
			locks[index].Lock()
			recordings[index] = append(recordings[index], packet.Samples...)
			locks[index].Unlock()
		}, func(err error) {
			select {
			case failures <- err:
			default:
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Second)
	collecting.Store(true)
	if output, err := exec.Command("afplay", wav).CombinedOutput(); err != nil {
		t.Fatalf("play speech: %v %s", err, output)
	}
	time.Sleep(250 * time.Millisecond)
	for _, capture := range captures {
		if err := capture.Stop(); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	levels := make([]float64, 0, 2)
	for index, samples := range recordings {
		if len(samples) < 4*SampleRate {
			t.Fatal("playback fixture/capture is too short")
		}
		// Allow the native canceller to converge before measuring suppression.
		levels = append(levels, RMS(samples[2*SampleRate:]))
		if directory := os.Getenv("DAYBEE_TEST_CAPTURE_DIR"); directory != "" {
			name := "raw.wav"
			if index == 1 {
				name = "echo-cancelled.wav"
			}
			writer, err := NewWavWriter(filepath.Join(directory, name))
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
	}
	if levels[0] < .0001 {
		t.Fatal("raw playback is too quiet to establish a meaningful speaker baseline")
	}
	attenuation := 20 * math.Log10(levels[0]/levels[1])
	t.Logf("speaker playback: raw RMS=%.6f processed RMS=%.6f total-level difference=%.1f dB", levels[0], levels[1], attenuation)
	// Apple voice processing can change the gain/noise floor. Total RMS is not
	// an echo metric; use TestEchoCancellationSpeechAcceptance on these files.
}

func TestVoiceCapturePreservesNearSpeech(t *testing.T) {
	if os.Getenv("DAYBEE_TEST_NEAR_SPEECH") != "1" {
		t.Skip("Set DAYBEE_TEST_NEAR_SPEECH=1 and speak into the microphone during this ten-second test")
	}
	capture, err := NewNativeCapture()
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	var mu sync.Mutex
	var samples []float32
	var collecting atomic.Bool
	gate := os.Getenv("DAYBEE_TEST_NEAR_SPEECH_START_FILE")
	collecting.Store(gate == "")
	failures := make(chan error, 1)
	if err := capture.Start(CaptureOptions{DeviceID: "default", EchoCancellation: true}, func(packet Packet) {
		if !collecting.Load() {
			return
		}
		mu.Lock()
		samples = append(samples, packet.Samples...)
		mu.Unlock()
	}, func(err error) {
		select {
		case failures <- err:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	if gate != "" {
		t.Log("READY: microphone initialized; waiting for speech-test start signal")
		deadline := time.Now().Add(time.Minute)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			if time.Now().After(deadline) {
				capture.Stop()
				t.Fatal("speech test start signal timed out")
			}
			time.Sleep(10 * time.Millisecond)
		}
		collecting.Store(true)
	}
	t.Log("SPEAK_NOW: speak into the microphone for the next ten seconds, including while a video plays")
	time.Sleep(10 * time.Second)
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	if len(samples) < 8*SampleRate {
		t.Fatal("not enough microphone samples")
	}
	level := RMS(samples[SampleRate:])
	t.Logf("near speech: RMS=%.6f samples=%d", level, len(samples))
	if directory := os.Getenv("DAYBEE_TEST_CAPTURE_DIR"); directory != "" {
		writer, err := NewWavWriter(filepath.Join(directory, "near-speech.wav"))
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
	if level < .0005 {
		t.Fatal("no audible near-end speech; do not count speaker suppression as acceptance")
	}
}
