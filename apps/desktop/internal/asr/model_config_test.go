package asr

import (
	"path/filepath"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/infer"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestOfflineConfigurationsUseCorrectModelArchitecture(t *testing.T) {
	for _, model := range asrmodel.All() {
		t.Run(string(model.ID), func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), model.Directory)
			config, err := offlineConfig(directory)
			if err != nil {
				t.Fatal(err)
			}
			if config.ModelConfig.Provider != "cpu" || config.ModelConfig.NumThreads != infer.Threads() {
				t.Fatal("CPU defaults changed")
			}
			var files []string
			switch model.ID {
			case protocol.ASRWhisperTiny, protocol.ASRWhisperBase, protocol.ASRWhisperSmall:
				w := config.ModelConfig.Whisper
				files = []string{w.Encoder, w.Decoder, config.ModelConfig.Tokens}
				if w.Language != "" || w.Task != "transcribe" || w.TailPaddings != -1 {
					t.Fatal("Whisper must auto-detect language and transcribe with native tail padding")
				}
				for i, file := range files {
					if filepath.Base(file) != model.Files[i] {
						t.Fatal("Whisper is not using the pinned INT8 assets", file)
					}
				}
			case protocol.ASRQwen3:
				q := config.ModelConfig.Qwen3ASR
				files = []string{q.ConvFrontend, q.Encoder, q.Decoder, q.Tokenizer}
				if config.FeatConfig.FeatureDim != 128 || q.Seed != 42 {
					t.Fatal("Qwen regression defaults changed")
				}
			case protocol.ASRFireRedAED:
				files = []string{config.ModelConfig.FireRedAsr.Encoder, config.ModelConfig.FireRedAsr.Decoder, config.ModelConfig.Tokens}
			case protocol.ASRFireRedCTC:
				files = []string{config.ModelConfig.FireRedAsrCtc.Model, config.ModelConfig.Tokens}
			case protocol.ASRSenseVoice:
				files = []string{config.ModelConfig.SenseVoice.Model, config.ModelConfig.Tokens}
				if config.ModelConfig.SenseVoice.UseInverseTextNormalization != 1 {
					t.Fatal("missing SenseVoice ITN")
				}
			default:
				t.Fatal("model has no architecture regression coverage")
			}
			if model.ID != protocol.ASRQwen3 && config.FeatConfig.FeatureDim != 80 {
				t.Fatal("wrong feature dimensions")
			}
			for _, file := range files {
				if file == "" || filepath.Dir(file) != directory {
					t.Fatal("wrong model asset path", file)
				}
			}
		})
	}
	if _, err := offlineConfig("unknown-model"); err == nil {
		t.Fatal("unknown model accepted")
	}
}
