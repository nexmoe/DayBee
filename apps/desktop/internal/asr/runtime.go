package asr

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
)

const Version = "1.13.8"
const OnnxRuntimeVersion = "1.28.2"

type Result struct{ Text, Language string }

// Start is the speech's sample offset since the last VAD reset, including
// silence. It must survive queuing and inference to retain capture time.
type Speech struct {
	Start   int
	Samples []float32
}

type Recognizer interface {
	Decode([]float32, int) (Result, error)
	Close()
}

type VAD interface {
	Accept([]float32)
	Detected() bool
	Pop() Speech
	Flush()
	Reset()
	Close()
}

type Runtime interface {
	NewRecognizer(string) (Recognizer, error)
	NewVAD(string) (VAD, error)
	ReadWave(string) ([]float32, int, error)
}

type Sherpa struct{}

func CheckVersion() error {
	actual := strings.TrimPrefix(sherpa.GetVersion(), "v")
	if actual != Version {
		return fmt.Errorf("sherpa native version %s does not match Go bindings %s", actual, Version)
	}
	if actual := sherpa.GetOnnxruntimeVersion(); actual != OnnxRuntimeVersion {
		return fmt.Errorf("ONNX Runtime %s does not match pinned %s", actual, OnnxRuntimeVersion)
	}
	return nil
}

func (Sherpa) NewRecognizer(directory string) (Recognizer, error) {
	if err := CheckVersion(); err != nil {
		return nil, err
	}
	config, err := offlineConfig(directory)
	if err != nil {
		return nil, err
	}
	r := sherpa.NewOfflineRecognizer(&config)
	if r == nil {
		return nil, fmt.Errorf("Failed to initialize ASR model: %s", directory)
	}
	slog.Info("ASR initialized", "sherpa", sherpa.GetVersion(), "onnxruntime", sherpa.GetOnnxruntimeVersion())
	return &offlineRecognizer{r}, nil
}

type offlineRecognizer struct{ native *sherpa.OfflineRecognizer }

func (r *offlineRecognizer) Decode(samples []float32, rate int) (Result, error) {
	if len(samples) == 0 {
		return Result{}, nil
	}
	if r.native == nil {
		return Result{}, errors.New("ASR recognizer is closed")
	}
	stream := sherpa.NewOfflineStream(r.native)
	if stream == nil {
		return Result{}, errors.New("Failed to create ASR stream.")
	}
	defer sherpa.DeleteOfflineStream(stream)
	stream.AcceptWaveform(rate, samples)
	r.native.Decode(stream)
	result := stream.GetResult()
	if result == nil {
		return Result{}, errors.New("ASR returned no result.")
	}
	return Result{strings.TrimSpace(result.Text), result.Lang}, nil
}

func (r *offlineRecognizer) Close() {
	if r.native != nil {
		sherpa.DeleteOfflineRecognizer(r.native)
		r.native = nil
	}
}

func (Sherpa) NewVAD(path string) (VAD, error) {
	if err := CheckVersion(); err != nil {
		return nil, err
	}
	config := sherpa.VadModelConfig{
		SampleRate: audio.SampleRate, NumThreads: 1, Provider: "cpu",
		SileroVad: sherpa.SileroVadModelConfig{
			Model: path, Threshold: .5, WindowSize: audio.WindowSize,
			MinSilenceDuration: .5, MinSpeechDuration: .25, MaxSpeechDuration: 15,
		},
	}
	vad := sherpa.NewVoiceActivityDetector(&config, 60)
	if vad == nil {
		return nil, errors.New("Failed to initialize Silero VAD.")
	}
	return &voiceActivityDetector{vad}, nil
}

type voiceActivityDetector struct{ native *sherpa.VoiceActivityDetector }

func (v *voiceActivityDetector) Accept(samples []float32) {
	if len(samples) > 0 {
		v.native.AcceptWaveform(samples)
	}
}
func (v *voiceActivityDetector) Detected() bool { return v.native.IsSpeech() }
func (v *voiceActivityDetector) Flush()         { v.native.Flush() }
func (v *voiceActivityDetector) Reset()         { v.native.Reset() }
func (v *voiceActivityDetector) Pop() Speech {
	if v.native.IsEmpty() {
		return Speech{}
	}
	segment := v.native.Front()
	v.native.Pop()
	if segment == nil {
		return Speech{}
	}
	return Speech{Start: segment.Start, Samples: segment.Samples}
}
func (v *voiceActivityDetector) Close() {
	if v.native != nil {
		sherpa.DeleteVoiceActivityDetector(v.native)
		v.native = nil
	}
}

func (Sherpa) ReadWave(path string) ([]float32, int, error) {
	wave := sherpa.ReadWave(path)
	if wave == nil {
		return nil, 0, fmt.Errorf("Unable to read audio file: %s", path)
	}
	return wave.Samples, wave.SampleRate, nil
}
