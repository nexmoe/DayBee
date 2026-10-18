//go:build !darwin

package audio

import (
	"errors"

	"github.com/gen2brain/malgo"
)

func newVoiceCapture(malgo.DeviceID, func([]float32), func(error)) (captureDevice, error) {
	return nil, errors.New("Echo cancellation is not available on this platform.")
}
