package protocol

import "github.com/nexmoe/eve/apps/desktop/internal/speechanalyzer"

type ASRModel string

const (
	ASRSpeechAnalyzer ASRModel = "speech-analyzer"
	ASRWhisperTiny    ASRModel = "whisper-tiny"
	ASRWhisperBase    ASRModel = "whisper-base"
	ASRWhisperSmall   ASRModel = "whisper-small"
	ASRQwen3          ASRModel = "qwen3-0.6b"
	ASRFireRedAED     ASRModel = "firered-asr2-aed"
	ASRFireRedCTC     ASRModel = "firered-asr2-ctc"
	ASRSenseVoice     ASRModel = "sensevoice-2025"
)

// Prefer Apple's system model on compatible Macs. Existing explicit selections
// survive normalization; other platforms and older Macs retain Whisper Tiny.
var DefaultASRModel = defaultASRModel(speechanalyzer.Available())

func defaultASRModel(speechAvailable bool) ASRModel {
	if speechAvailable {
		return ASRSpeechAnalyzer
	}
	return ASRWhisperTiny
}

func ValidASRModel(model ASRModel) bool {
	switch model {
	case ASRSpeechAnalyzer:
		return speechanalyzer.Available()
	case ASRWhisperTiny, ASRWhisperBase, ASRWhisperSmall, ASRQwen3, ASRFireRedAED, ASRFireRedCTC, ASRSenseVoice:
		return true
	}
	return false
}

type ASRModelSnapshot struct {
	ID   ASRModel `json:"id"`
	Name string   `json:"name"`
	// EstimatedBytes is the compressed download size, not resident memory.
	// Zero for OS-managed SpeechAnalyzer assets, whose size is not fixed.
	EstimatedBytes int64 `json:"estimatedBytes"`
	Ready          bool  `json:"ready"`
}

type ASRModelsSnapshot struct {
	Items            []ASRModelSnapshot `json:"items"`
	Busy             bool               `json:"busy"`
	DownloadingModel *ASRModel          `json:"downloadingModel"`
}
