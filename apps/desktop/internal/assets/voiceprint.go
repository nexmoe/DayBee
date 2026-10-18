package assets

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

func (m *Manager) VoiceprintPath() string {
	for _, root := range m.roots {
		path := filepath.Join(root, voiceprint.ModelName)
		if usableFile(path) {
			return path
		}
	}
	return filepath.Join(m.root, voiceprint.ModelName)
}

func (m *Manager) EnsureVoiceprint(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	return m.withGate(ctx, m.voiceGate, func() error {
		if usableFile(m.VoiceprintPath()) {
			return nil
		}
		url := "https://github.com/k2-fsa/sherpa-onnx/releases/download/speaker-recongition-models/" + voiceprint.ModelName
		return m.download(ctx, url, m.VoiceprintPath(), "Downloading voiceprint model")
	})
}
