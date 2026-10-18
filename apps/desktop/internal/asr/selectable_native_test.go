package asr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/DayBee/apps/desktop/internal/assets"
)

// Opt-in integration downloads the pinned real models into an isolated cache.
// Ordinary tests use small archive fixtures and never access the network.
func TestNativeSelectableModels(t *testing.T) {
	root, audio := os.Getenv("DAYBEE_TEST_SELECTABLE_ASR_ROOT"), os.Getenv("DAYBEE_TEST_AUDIO_DIR")
	if root == "" || audio == "" {
		t.Skip("Set DAYBEE_TEST_SELECTABLE_ASR_ROOT and DAYBEE_TEST_AUDIO_DIR for real model downloads and inference")
	}
	var legacy []string
	if previous := os.Getenv("DAYBEE_TEST_LEGACY_MODEL_ROOT"); previous != "" {
		legacy = append(legacy, previous)
	}
	manager := assets.New(root, legacy, nil)
	samples, rate, err := (Sherpa{}).ReadWave(filepath.Join(audio, "chinese.wav"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	for _, model := range asrmodel.All() {
		t.Run(string(model.ID), func(t *testing.T) {
			if err := manager.EnsureASRModel(ctx, model.ID, "zh"); err != nil {
				t.Fatal(err)
			}
			manager.SelectASRModel(model.ID, "zh")
			recognizer, err := (Sherpa{}).NewRecognizer(manager.ASRPath())
			if err != nil {
				t.Fatal(err)
			}
			defer recognizer.Close()
			started := time.Now()
			result, err := recognizer.Decode(samples, rate)
			if err != nil || result.Text == "" {
				t.Fatal("empty or failed real model inference", err)
			}
			t.Logf("%s decoded %.2fs audio in %s: %s", model.Name, float64(len(samples))/float64(rate), time.Since(started), result.Text)
		})
	}
}
