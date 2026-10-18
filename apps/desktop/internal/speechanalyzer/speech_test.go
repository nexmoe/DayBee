package speechanalyzer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestModelKeysPinLanguageAndRejectDirectories(t *testing.T) {
	for _, language := range []string{"auto", "zh", "en-US", "ja"} {
		key := ModelKey(language)
		if !IsModelKey(key) || Language(key) != language {
			t.Fatal("model key did not retain its language", key)
		}
	}
	if ModelKey("") != ModelKey("auto") || IsModelKey("/models/speech-analyzer") {
		t.Fatal("model keys are distinct from downloaded directories")
	}
}

func TestDecodeEmptyInvalidAndCanceledInput(t *testing.T) {
	if result, err := Decode(context.Background(), "en", nil, 16000); err != nil || result.Text != "" {
		t.Fatal("empty input should not load native models", result, err)
	}
	if _, err := Decode(context.Background(), "en", []float32{0}, 0); err == nil {
		t.Fatal("invalid rate accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ensure(ctx, "en"); !errors.Is(err, context.Canceled) {
		t.Fatal("preparation ignored cancellation", err)
	}
	if _, err := Decode(ctx, "en", []float32{0}, 16000); !errors.Is(err, context.Canceled) {
		t.Fatal("decode ignored cancellation", err)
	}
}

func TestNativeSystemInventory(t *testing.T) {
	if !Available() {
		t.Skip("SpeechAnalyzer is unavailable on this platform or the bridge is not built")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := request(ctx, "status", "en", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SpeechAnalyzer available; English assets installed: %v", result.Ready)
	if _, err := request(ctx, "status", "not-a-language", nil, 0); err == nil {
		t.Fatal("unsupported language must produce an actionable error")
	}
}

// The normal suite never downloads OS speech assets or opens a microphone.
func TestNativeSystemPreparationAndSilence(t *testing.T) {
	if os.Getenv("DAYBEE_TEST_SPEECH_ANALYZER") != "1" || !Available() {
		t.Skip("Set DAYBEE_TEST_SPEECH_ANALYZER=1 to prepare the real system model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := Ensure(ctx, "en"); err != nil {
		t.Fatal(err)
	}
	if !Ready("en") {
		t.Fatal("prepared system assets are not ready")
	}
	result, err := Decode(ctx, "en", make([]float32, 16000), 16000)
	if err != nil || result.Text != "" {
		t.Fatal("silence should produce no transcript", result, err)
	}
}
