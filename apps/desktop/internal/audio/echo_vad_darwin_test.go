package audio_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/codec"
)

// A whole-file decode can return empty while the production VAD extracts short
// residual fragments that ASR transcribes. Exercise the same segmentation here.
func TestEchoCancellationVADAcceptance(t *testing.T) {
	model, vadPath := os.Getenv("DAYBEE_TEST_MODEL_DIR"), os.Getenv("DAYBEE_TEST_VAD_PATH")
	directory := os.Getenv("DAYBEE_TEST_SPEAKER_CAPTURE_DIR")
	near, nearExpected := os.Getenv("DAYBEE_TEST_NEAR_CAPTURE_WAV"), os.Getenv("DAYBEE_TEST_NEAR_EXPECTED")
	playbackExpected := os.Getenv("DAYBEE_TEST_PLAYBACK_EXPECTED")
	if model == "" || vadPath == "" || directory == "" || near == "" || nearExpected == "" || playbackExpected == "" {
		t.Skip("Provide model, VAD, speaker and near captures, and expected control text for residual echo acceptance")
	}
	control := os.Getenv("DAYBEE_TEST_PLAYBACK_CONTROL_WAV")
	if control == "" {
		control = filepath.Join(directory, "raw.wav")
	}
	recognizer, err := (asr.Sherpa{}).NewRecognizer(model)
	if err != nil {
		t.Fatal(err)
	}
	defer recognizer.Close()
	transcripts := map[string][]string{}
	cases := []struct{ name, path, expected string }{
		{"control", control, playbackExpected},
		{"processed", filepath.Join(directory, "echo-cancelled.wav"), ""},
		{"near", near, nearExpected},
	}
	if path := os.Getenv("DAYBEE_TEST_NEAR_CONTROL_WAV"); path != "" {
		cases = append(cases, struct{ name, path, expected string }{"nearControl", path, nearExpected})
	}
	for _, trial := range cases {
		samples, rate, err := codec.ReadMono(context.Background(), trial.path)
		if err != nil || rate != audio.SampleRate {
			t.Fatal("invalid capture", trial.path, rate, err)
		}
		vad, err := (asr.Sherpa{}).NewVAD(vadPath)
		if err != nil {
			t.Fatal(err)
		}
		transcripts[trial.name] = []string{}
		decode := func() {
			for {
				speech := vad.Pop()
				if len(speech.Samples) == 0 {
					return
				}
				result, err := recognizer.Decode(speech.Samples, rate)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s VAD fragment %.2fs at %.2fs: %q", trial.name,
					float64(len(speech.Samples))/float64(rate), float64(speech.Start)/float64(rate), result.Text)
				if strings.TrimSpace(result.Text) != "" {
					transcripts[trial.name] = append(transcripts[trial.name], result.Text)
				}
			}
		}
		for offset := 0; offset < len(samples); offset += audio.WindowSize {
			window := make([]float32, audio.WindowSize)
			copy(window, samples[offset:min(offset+audio.WindowSize, len(samples))])
			vad.Accept(window)
			decode()
		}
		vad.Flush()
		decode()
		vad.Close()
		text := strings.Join(transcripts[trial.name], " ")
		if trial.name == "processed" && text != "" {
			t.Errorf("residual speaker playback produced VAD/ASR transcripts: %q", text)
		} else if trial.name != "processed" && (text == "" || !strings.Contains(text, trial.expected)) {
			t.Errorf("%s speech control was not preserved: %q", trial.name, text)
		}
	}
	if path := os.Getenv("DAYBEE_TEST_ECHO_REPORT"); path != "" {
		raw, _ := json.MarshalIndent(transcripts, "", "  ")
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
