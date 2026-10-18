package asr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
)

func TestNativeVersions(t *testing.T) {
	if err := CheckVersion(); err != nil {
		t.Fatal(err)
	}
	t.Log("sherpa", sherpa.GetVersion(), "ONNX Runtime", sherpa.GetOnnxruntimeVersion())
}

func TestNativeVAD(t *testing.T) {
	path := os.Getenv("DAYBEE_TEST_VAD_PATH")
	if path == "" {
		t.Skip("Set DAYBEE_TEST_VAD_PATH for real Silero VAD integration")
	}
	for range 2 {
		vad, err := (Sherpa{}).NewVAD(path)
		if err != nil {
			t.Fatal(err)
		}
		for range 40 {
			vad.Accept(make([]float32, 512))
		}
		vad.Flush()
		if vad.Detected() || len(vad.Pop().Samples) != 0 {
			t.Fatal("Silero classified digital silence as speech")
		}
		vad.Reset()
		vad.Close()
		vad.Close()
	}
}

func TestNativeVADRetainsSpeechOffsetsAcrossSilenceAndReset(t *testing.T) {
	path, fixtures := os.Getenv("DAYBEE_TEST_VAD_PATH"), os.Getenv("DAYBEE_TEST_AUDIO_DIR")
	if path == "" || fixtures == "" {
		t.Skip("Set DAYBEE_TEST_VAD_PATH and DAYBEE_TEST_AUDIO_DIR for native speech timing")
	}
	samples, rate, err := (Sherpa{}).ReadWave(filepath.Join(fixtures, "chinese.wav"))
	if err != nil || rate != audio.SampleRate {
		t.Fatal("speech timing requires a 16 kHz fixture", rate, err)
	}
	input := append(make([]float32, 4*rate), samples...)
	input = append(input, make([]float32, rate)...)
	vad, err := (Sherpa{}).NewVAD(path)
	if err != nil {
		t.Fatal(err)
	}
	defer vad.Close()
	firstOffset := -1
	for range 2 {
		for offset := 0; offset < len(input); offset += audio.WindowSize {
			window := make([]float32, audio.WindowSize)
			copy(window, input[offset:min(offset+audio.WindowSize, len(input))])
			vad.Accept(window)
		}
		vad.Flush()
		count, previousEnd := 0, 0
		for {
			speech := vad.Pop()
			if len(speech.Samples) == 0 {
				break
			}
			if count == 0 {
				if firstOffset < 0 {
					firstOffset = speech.Start
				} else if speech.Start != firstOffset {
					t.Fatalf("VAD reset did not reset the sample origin: %d != %d", speech.Start, firstOffset)
				}
			}
			if speech.Start < 3*rate || speech.Start < previousEnd || speech.Start+len(speech.Samples) > len(input)+audio.WindowSize {
				t.Fatalf("speech offsets lost silence or ordering: start=%d previousEnd=%d length=%d", speech.Start, previousEnd, len(speech.Samples))
			}
			previousEnd = speech.Start + len(speech.Samples)
			count++
		}
		if count == 0 {
			t.Fatal("native VAD did not recognize the speech fixture")
		}
		vad.Reset()
	}
}

// CI and release acceptance opt in with the exact cached model and fixed WAV
// fixtures. Ordinary unit tests never fetch gigabytes or access microphones.
func TestNativeInference(t *testing.T) {
	model := os.Getenv("DAYBEE_TEST_MODEL_DIR")
	if model == "" {
		t.Skip("Set DAYBEE_TEST_MODEL_DIR for native Qwen3 inference regression")
	}
	fixtures := os.Getenv("DAYBEE_TEST_AUDIO_DIR")
	if fixtures == "" {
		t.Fatal("DAYBEE_TEST_AUDIO_DIR must contain fixed regression WAVs")
	}
	runtime := Sherpa{}
	initialized := time.Now()
	recognizer, err := runtime.NewRecognizer(model)
	if err != nil {
		t.Fatal(err)
	}
	defer recognizer.Close()
	type entry struct {
		File         string  `json:"file"`
		SHA256       string  `json:"sha256"`
		Samples      int     `json:"samples"`
		Rate         int     `json:"rate"`
		Milliseconds float64 `json:"milliseconds"`
		Text         string  `json:"text"`
		Language     string  `json:"language"`
	}
	report := struct {
		Sherpa                     string  `json:"sherpa"`
		OnnxRuntime                string  `json:"onnxruntime"`
		Target                     string  `json:"target"`
		InitializationMilliseconds float64 `json:"initializationMilliseconds"`
		Results                    []entry `json:"results"`
	}{Version, OnnxRuntimeVersion, targetName(), float64(time.Since(initialized).Microseconds()) / 1000, []entry{}}
	files, err := filepath.Glob(filepath.Join(fixtures, "*.wav"))
	if err != nil || len(files) < 4 {
		t.Fatal("Need fixed Chinese, silence, short and long WAV samples")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			samples, rate, err := runtime.ReadWave(path)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			first, err := recognizer.Decode(samples, rate)
			if err != nil {
				t.Fatal(err)
			}
			duration := time.Since(start)
			t.Logf("samples=%d rate=%d duration=%s text=%q", len(samples), rate, duration, first.Text)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			report.Results = append(report.Results, entry{filepath.Base(path), hex.EncodeToString(hash[:]), len(samples), rate,
				float64(duration.Microseconds()) / 1000, first.Text, first.Language})
			second, err := recognizer.Decode(samples, rate)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatal("seed 42 did not reproduce output", first, second)
			}
		})
	}
	if path := os.Getenv("DAYBEE_TEST_ASR_REPORT"); path != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func targetName() string { return runtime.GOOS + "-" + runtime.GOARCH }
