package voiceprint

import (
	"os"
	"testing"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// Opt-in, offline verification. No microphone or saved user enrollment is used.
func TestNativeSpeakerVerification(t *testing.T) {
	model, positive, negative := os.Getenv("DAYBEE_TEST_VOICEPRINT_MODEL"), os.Getenv("DAYBEE_TEST_VOICEPRINT_SPEECH"), os.Getenv("DAYBEE_TEST_VOICEPRINT_OTHER")
	if model == "" || positive == "" || negative == "" {
		t.Skip("Set voiceprint model and two different-speaker WAV fixtures")
	}
	m, err := New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	wave := sherpa.ReadWave(positive)
	if wave == nil || len(wave.Samples) < 32000 {
		t.Fatal("positive fixture must contain at least two seconds")
	}
	start := time.Now()
	mid := len(wave.Samples) / 2
	vector, err := m.Embed(wave.Samples[:mid], wave.SampleRate)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save([][]float32{vector}); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Match(m.Snapshot(), wave.Samples[mid:], wave.SampleRate, nil); got != "me" {
		t.Fatal("same-speaker continuation was not recognized")
	}
	other := sherpa.ReadWave(negative)
	if other == nil {
		t.Fatal("missing negative fixture")
	}
	if got, _ := m.Match(m.Snapshot(), other.Samples, other.SampleRate, nil); got != "" {
		t.Fatal("different speaker was mislabeled")
	}
	if got, _ := m.Match(m.Snapshot(), wave.Samples[:160], wave.SampleRate, nil); got != "" {
		t.Fatal("a short uncertain fragment was labeled")
	}
	t.Logf("Native enrollment and same/different-speaker matching: %s", time.Since(start))
}
