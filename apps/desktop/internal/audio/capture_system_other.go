//go:build !darwin

package audio

import "errors"

func newSystemCapture(func([]float32), func(error)) (captureDevice, error) {
	return nil, errors.New("System audio recording is currently supported on macOS only")
}

func SystemAudioSettingsURL() string { return "" }
