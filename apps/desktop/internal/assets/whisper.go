package assets

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

// Official Whisper archives include both FP32 and INT8 weights. Only prune the
// unused pair in a validated staging tree; existing caches are never modified.
func pruneWhisperFP32(spec asrmodel.Model, directory string) error {
	switch spec.ID {
	case protocol.ASRWhisperTiny, protocol.ASRWhisperBase, protocol.ASRWhisperSmall:
	default:
		return nil
	}
	for _, name := range spec.Files {
		if strings.HasSuffix(name, ".int8.onnx") {
			unused := strings.TrimSuffix(name, ".int8.onnx") + ".onnx"
			if err := os.Remove(filepath.Join(directory, unused)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
