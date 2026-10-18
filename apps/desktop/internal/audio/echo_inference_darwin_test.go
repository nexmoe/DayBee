package audio_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/codec"
)

// Acceptance needs both known speaker playback and real near-end speech.
// Digital silence alone must never qualify as successful echo cancellation.
func TestEchoCancellationSpeechAcceptance(t *testing.T) {
	model := os.Getenv("DAYBEE_TEST_MODEL_DIR")
	directory := os.Getenv("DAYBEE_TEST_SPEAKER_CAPTURE_DIR")
	near := os.Getenv("DAYBEE_TEST_NEAR_CAPTURE_WAV")
	playbackExpected := os.Getenv("DAYBEE_TEST_PLAYBACK_EXPECTED")
	nearExpected := os.Getenv("DAYBEE_TEST_NEAR_EXPECTED")
	if model == "" || directory == "" || near == "" || playbackExpected == "" || nearExpected == "" {
		t.Skip("Provide model, paired speaker captures, near speech, and expected speech fragments for native AEC acceptance")
	}
	recognizer, err := (asr.Sherpa{}).NewRecognizer(model)
	if err != nil {
		t.Fatal(err)
	}
	defer recognizer.Close()
	decode := func(path string) string {
		t.Helper()
		samples, rate, err := codec.ReadMono(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := recognizer.Decode(samples, rate)
		if err != nil {
			t.Fatal(err)
		}
		return result.Text
	}
	if raw := decode(filepath.Join(directory, "raw.wav")); !strings.Contains(raw, playbackExpected) {
		t.Fatalf("speaker control did not contain the known playback: %q", raw)
	}
	if processed := decode(filepath.Join(directory, "echo-cancelled.wav")); strings.TrimSpace(processed) != "" {
		t.Fatalf("speaker playback still produced a transcript: %q", processed)
	}
	if live := decode(near); !strings.Contains(live, nearExpected) {
		t.Fatalf("near-end speech was lost or changed: %q", live)
	}
}
