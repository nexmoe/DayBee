package asr

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

func TestSpeechRecognizerClosedAndEmptyInput(t *testing.T) {
	r := &speechRecognizer{language: "en"}
	if result, err := r.Decode(nil, 16000); err != nil || result.Text != "" {
		t.Fatal("empty input loaded a system model", err)
	}
	r.Close()
	r.Close()
	if _, err := r.Decode([]float32{0}, 16000); err == nil {
		t.Fatal("closed recognizer accepted audio")
	}
}

func TestNativeSpeechAnalyzerTranscription(t *testing.T) {
	path := os.Getenv("DAYBEE_TEST_SPEECH_WAV")
	if path == "" || !speechanalyzer.Available() {
		t.Skip("Set DAYBEE_TEST_SPEECH_WAV to a fixed English WAV to test system transcription")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := speechanalyzer.Ensure(ctx, "en"); err != nil {
		t.Fatal(err)
	}
	samples, rate, err := (Sherpa{}).ReadWave(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r, err := (Sherpa{}).NewRecognizer(speechanalyzer.ModelKey("en"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Decode(samples, rate)
		r.Close()
		if err != nil || strings.TrimSpace(result.Text) == "" || result.Language != "en" {
			t.Fatal("native system transcription failed", result, err)
		}
		if expected := os.Getenv("DAYBEE_TEST_SPEECH_EXPECT"); expected != "" && !strings.Contains(strings.ToLower(result.Text), strings.ToLower(expected)) {
			t.Fatal("transcription did not contain the expected phrase", result.Text, expected)
		}
		t.Logf("SpeechAnalyzer decoded %.2fs at %d Hz: %s", float64(len(samples))/float64(rate), rate, result.Text)
	}
}
