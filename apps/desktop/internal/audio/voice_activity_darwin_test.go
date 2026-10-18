package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/permissions"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// Verify the independent detector with live speech before using it to filter
// audio. A detector stuck at zero must never qualify as echo suppression.
func TestVoiceActivityLive(t *testing.T) {
	mode := os.Getenv("DAYBEE_TEST_VOICE_ACTIVITY")
	if mode != "near" && mode != "speaker" {
		t.Skip("Set DAYBEE_TEST_VOICE_ACTIVITY=near or speaker for live detector verification")
	}
	requireRecorderStopped(t)
	permission := permissions.Status()
	t.Log("microphone permission:", permission.State)
	if permission.State != protocol.PermissionAuthorized {
		t.Fatal("the test process is not authorized to capture microphone audio")
	}
	var mu sync.Mutex
	var samples, ungated []float32
	var counts [3]int
	var collecting atomic.Bool
	failures := make(chan error, 1)
	capture := &voiceCapture{
		detectSpeech: true,
		consume: func(packet []float32) {
			if collecting.Load() {
				mu.Lock()
				samples = append(samples, packet...)
				mu.Unlock()
			}
		},
		inspect: func(packet []float32, activity int) {
			if collecting.Load() {
				mu.Lock()
				ungated = append(ungated, packet...)
				counts[activity+1]++
				mu.Unlock()
			}
		},
		onError: func(err error) {
			select {
			case failures <- err:
			default:
			}
		},
	}
	if err := capture.Start(); err != nil {
		t.Fatal(err)
	}
	defer capture.Stop()
	if gate := os.Getenv("DAYBEE_TEST_NEAR_SPEECH_START_FILE"); gate != "" {
		if err := os.WriteFile(gate+".ready", []byte("ready\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("READY: detector initialized; waiting for speech-test start signal")
		deadline := time.Now().Add(time.Minute)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("speech-test start signal timed out")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	collecting.Store(true)
	if os.Getenv("DAYBEE_TEST_NEAR_SPEECH_CUE") == "1" {
		if err := exec.Command("/usr/bin/say", "-v", "Tingting", "录音已开始，请说测试句。").Run(); err != nil {
			t.Fatal(err)
		}
	}
	var playback *exec.Cmd
	if mode == "speaker" {
		wav := os.Getenv("DAYBEE_TEST_SPEAKER_WAV")
		if wav == "" {
			t.Fatal("speaker test requires DAYBEE_TEST_SPEAKER_WAV")
		}
		playback = exec.Command("afplay", wav)
		if err := playback.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = playback.Process.Kill(); _ = playback.Wait() })
	}
	if finish := os.Getenv("DAYBEE_TEST_NEAR_SPEECH_STOP_FILE"); finish != "" {
		deadline := time.Now().Add(2 * time.Minute)
		for {
			if _, err := os.Stat(finish); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Error("speech-test completion signal timed out")
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	} else {
		seconds := 10
		if value := os.Getenv("DAYBEE_TEST_CAPTURE_SECONDS"); value != "" {
			var err error
			seconds, err = strconv.Atoi(value)
			if err != nil || seconds < 1 || seconds > 120 {
				t.Fatal("DAYBEE_TEST_CAPTURE_SECONDS must be between 1 and 120")
			}
		}
		time.Sleep(time.Duration(seconds) * time.Second)
	}
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	requireRecorderStopped(t)
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	if directory := os.Getenv("DAYBEE_TEST_CAPTURE_DIR"); directory != "" {
		name := "near-speech.wav"
		if mode == "speaker" {
			name = "echo-cancelled.wav"
		}
		for file, audio := range map[string][]float32{name: samples, "ungated.wav": ungated} {
			writer, err := NewWavWriter(filepath.Join(directory, file))
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Append(audio); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("detector: unavailable=%d inactive=%d active=%d samples=%d RMS=%.6f ungatedRMS=%.6f", counts[0], counts[1], counts[2], len(samples), RMS(samples), RMS(ungated))
	if counts[0] > 0 || (mode == "near" && counts[2] == 0) || (mode == "speaker" && counts[2] > 0) {
		t.Fatal("voice detector did not distinguish the live speech control from speaker playback")
	}
}

func requireRecorderStopped(t *testing.T) {
	t.Helper()
	output, err := exec.Command("/usr/bin/pgrep", "-x", "DayBee").Output()
	if err == nil {
		t.Fatalf("DayBee is running (PID %s); concurrent voice-processing capture invalidates this test", output)
	}
	if code, ok := err.(*exec.ExitError); !ok || code.ExitCode() != 1 {
		t.Fatalf("check for a conflicting recording instance: %v", err)
	}
}
