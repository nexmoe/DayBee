package asr

import (
	"fmt"
	"path/filepath"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/infer"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func offlineConfig(directory string) (sherpa.OfflineRecognizerConfig, error) {
	model, ok := asrmodel.ForDirectory(directory)
	if !ok {
		return sherpa.OfflineRecognizerConfig{}, fmt.Errorf("Unknown ASR model directory: %s", directory)
	}
	file := func(name string) string { return filepath.Join(directory, name) }
	config := sherpa.OfflineRecognizerConfig{
		FeatConfig:  sherpa.FeatureConfig{SampleRate: audio.SampleRate, FeatureDim: 80},
		ModelConfig: sherpa.OfflineModelConfig{NumThreads: infer.Threads(), Provider: "cpu"},
	}
	switch model.ID {
	case protocol.ASRWhisperTiny, protocol.ASRWhisperBase, protocol.ASRWhisperSmall:
		prefix := strings.TrimPrefix(string(model.ID), "whisper-")
		config.ModelConfig.Whisper = sherpa.OfflineWhisperModelConfig{
			Encoder: file(prefix + "-encoder.int8.onnx"), Decoder: file(prefix + "-decoder.int8.onnx"),
			Language: "", Task: "transcribe", TailPaddings: -1,
		}
		config.ModelConfig.Tokens = file(prefix + "-tokens.txt")
	case protocol.ASRQwen3:
		config.FeatConfig.FeatureDim = 128
		config.ModelConfig.Qwen3ASR = sherpa.OfflineQwen3ASRModelConfig{
			ConvFrontend: file("conv_frontend.onnx"), Encoder: file("encoder.int8.onnx"),
			Decoder: file("decoder.int8.onnx"), Tokenizer: file("tokenizer"), Seed: 42,
		}
	case protocol.ASRFireRedAED:
		config.ModelConfig.FireRedAsr = sherpa.OfflineFireRedAsrModelConfig{Encoder: file("encoder.int8.onnx"), Decoder: file("decoder.int8.onnx")}
		config.ModelConfig.Tokens = file("tokens.txt")
	case protocol.ASRFireRedCTC:
		config.ModelConfig.FireRedAsrCtc = sherpa.OfflineFireRedAsrCtcModelConfig{Model: file("model.int8.onnx")}
		config.ModelConfig.Tokens = file("tokens.txt")
	case protocol.ASRSenseVoice:
		config.ModelConfig.SenseVoice = sherpa.OfflineSenseVoiceModelConfig{Model: file("model.int8.onnx"), Language: "auto", UseInverseTextNormalization: 1}
		config.ModelConfig.Tokens = file("tokens.txt")
	}
	return config, nil
}
