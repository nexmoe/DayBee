// Package asrmodel pins the model assets used by download and inference.
package asrmodel

import (
	"os"
	"path/filepath"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

const QwenDirectory = "sherpa-onnx-qwen3-asr-0.6B-int8-2026-03-25"

type Model struct {
	ID             protocol.ASRModel
	Name           string
	Directory      string
	EstimatedBytes int64
	Files          []string
}

var catalog = []Model{
	{protocol.ASRWhisperTiny, "Whisper Tiny", "sherpa-onnx-whisper-tiny", 116_204_861,
		[]string{"tiny-encoder.int8.onnx", "tiny-decoder.int8.onnx", "tiny-tokens.txt"}},
	{protocol.ASRWhisperBase, "Whisper Base", "sherpa-onnx-whisper-base", 207_557_382,
		[]string{"base-encoder.int8.onnx", "base-decoder.int8.onnx", "base-tokens.txt"}},
	{protocol.ASRSenseVoice, "SenseVoice Small (2025)", "sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2025-09-09", 165_783_878,
		[]string{"model.int8.onnx", "tokens.txt"}},
	{protocol.ASRWhisperSmall, "Whisper Small", "sherpa-onnx-whisper-small", 639_387_718,
		[]string{"small-encoder.int8.onnx", "small-decoder.int8.onnx", "small-tokens.txt"}},
	{protocol.ASRFireRedCTC, "FireRedASR2 CTC", "sherpa-onnx-fire-red-asr2-ctc-zh_en-int8-2026-02-25", 520_516_278,
		[]string{"model.int8.onnx", "tokens.txt"}},
	{protocol.ASRQwen3, "Qwen3 ASR 0.6B", QwenDirectory, 878_702_423,
		[]string{"conv_frontend.onnx", "encoder.int8.onnx", "decoder.int8.onnx", "tokenizer/vocab.json", "tokenizer/merges.txt", "tokenizer/tokenizer_config.json"}},
	{protocol.ASRFireRedAED, "FireRedASR2 AED", "sherpa-onnx-fire-red-asr2-zh_en-int8-2026-02-26", 838_589_068,
		[]string{"encoder.int8.onnx", "decoder.int8.onnx", "tokens.txt"}},
}

var systemModel = Model{ID: protocol.ASRSpeechAnalyzer, Name: "SpeechAnalyzer"}

func All() []Model {
	models := make([]Model, 0, len(catalog)+1)
	if speechanalyzer.Available() {
		models = append(models, systemModel)
	}
	return append(models, catalog...)
}

func Find(id protocol.ASRModel) (Model, bool) {
	if id == protocol.ASRSpeechAnalyzer {
		return systemModel, true
	}
	for _, model := range catalog {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

func Name(id protocol.ASRModel) string {
	if model, ok := Find(id); ok {
		return model.Name
	}
	model, _ := Find(protocol.DefaultASRModel)
	return model.Name
}

func Backend(id protocol.ASRModel) string {
	if id == protocol.ASRSpeechAnalyzer {
		return "speech-analyzer"
	}
	return "sherpa-onnx"
}

func ForDirectory(directory string) (Model, bool) {
	for _, model := range catalog {
		if model.Directory == filepath.Base(filepath.Clean(directory)) {
			return model, true
		}
	}
	return Model{}, false
}

func (m Model) Ready(directory string) bool {
	if m.ID == protocol.ASRSpeechAnalyzer || len(m.Files) == 0 {
		return false // System readiness comes from AssetInventory, never a directory.
	}
	for _, name := range m.Files {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}
