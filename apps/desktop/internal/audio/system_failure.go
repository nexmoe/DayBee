package audio

import (
	"errors"
	"fmt"
	"strings"
)

func systemFailureError(failure int, detail string) error {
	detail = strings.TrimSpace(detail)
	switch failure {
	case 2:
		return errors.New("System audio buffer overflow. Recording was stopped safely.")
	case 3:
		return errors.New("System audio conversion failed. Recording was stopped safely.")
	case 4:
		if detail != "" {
			return fmt.Errorf("System audio capture was stopped by the user: %s", detail)
		}
		return errors.New("System audio capture was stopped by the user.")
	default:
		if detail != "" {
			return fmt.Errorf("%w %s", ErrInterrupted, detail)
		}
		return fmt.Errorf("%w System audio stream stopped", ErrInterrupted)
	}
}
