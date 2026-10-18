package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
)

// Settings are imported by config.Open; models remain readable in legacy roots.
// Copy the small state files once, without overwriting DayBee data or moving
// recordings. The marker prevents a cleared voiceprint from being resurrected.
func importLegacyState(root string, legacy []string) error {
	if len(legacy) == 0 {
		return nil
	}
	marker := filepath.Join(root, "daybee-migration.json")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, old := range legacy {
		paths := []string{filepath.Join(old, "voiceprint", "profile.json")}
		days, err := filepath.Glob(filepath.Join(old, "microphone-activity", "????????.json"))
		if err != nil {
			return err
		}
		paths = append(paths, days...)
		for _, source := range paths {
			relative, err := filepath.Rel(old, source)
			if err != nil {
				return err
			}
			if err := copyLegacyStateFile(source, filepath.Join(root, relative)); err != nil {
				return err
			}
		}
	}
	return config.WriteJSON(marker, map[string]int{"version": 1})
}

func copyLegacyStateFile(source, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return err
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".daybee-import-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(raw)
	err = errors.Join(writeErr, temporary.Sync(), temporary.Close())
	if err != nil {
		return err
	}
	// A link publishes a complete file atomically and fails if another instance
	// already created the destination. Both paths are on the same filesystem.
	err = os.Link(temporary.Name(), destination)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	return err
}
