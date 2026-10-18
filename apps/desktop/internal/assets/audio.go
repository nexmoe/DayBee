package assets

import (
	"context"

	"github.com/nexmoe/eve/apps/desktop/internal/codec"
)

func (m *Manager) Transcode(ctx context.Context, wavPath, flacPath string) error {
	return codec.EncodeFLAC(ctx, wavPath, flacPath)
}

func (m *Manager) ReadAudio(ctx context.Context, path string) ([]float32, int, error) {
	return codec.ReadMono(ctx, path)
}
