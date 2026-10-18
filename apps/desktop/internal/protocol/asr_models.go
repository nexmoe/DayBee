package protocol

type ASRModel string

const (
	ASRWhisperTiny  ASRModel = "whisper-tiny"
	ASRWhisperBase  ASRModel = "whisper-base"
	ASRWhisperSmall ASRModel = "whisper-small"
	ASRQwen3        ASRModel = "qwen3-0.6b"
	ASRFireRedAED   ASRModel = "firered-asr2-aed"
	ASRFireRedCTC   ASRModel = "firered-asr2-ctc"
	ASRSenseVoice   ASRModel = "sensevoice-2025"
)

// New installations and incomplete settings favor the smallest ASR model.
// Explicit selections remain unchanged across restarts and migrations.
const DefaultASRModel = ASRWhisperTiny

func ValidASRModel(model ASRModel) bool {
	switch model {
	case ASRWhisperTiny, ASRWhisperBase, ASRWhisperSmall, ASRQwen3, ASRFireRedAED, ASRFireRedCTC, ASRSenseVoice:
		return true
	}
	return false
}

type ASRModelSnapshot struct {
	ID   ASRModel `json:"id"`
	Name string   `json:"name"`
	// EstimatedBytes is the compressed download size, not resident memory.
	EstimatedBytes int64 `json:"estimatedBytes"`
	Ready          bool  `json:"ready"`
}

type ASRModelsSnapshot struct {
	Items            []ASRModelSnapshot `json:"items"`
	Busy             bool               `json:"busy"`
	DownloadingModel *ASRModel          `json:"downloadingModel"`
}
