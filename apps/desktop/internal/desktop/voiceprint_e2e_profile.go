package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

// loadTestVoiceprint is only used by the DAYBEE_E2E_TEST engine branch. It loads
// a profile the test already wrote and leaves a normal empty store unloaded.
func (s *DesktopService) loadTestVoiceprint(root string) error {
	path := filepath.Join(root, "voiceprint", "profile.json")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	manager, err := voiceprint.New(root, nil)
	s.voiceprints = manager
	if err != nil {
		return err
	}
	s.syncVoiceprintAdaptation()
	return nil
}
