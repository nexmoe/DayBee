package voiceprint

import (
	"os"
	"path/filepath"
	"testing"
)

// A file in place of the directory fails writes on Windows and Unix alike.
// Preserve and restore the original profile so failure tests retain disk evidence.
func blockProfileStorage(t *testing.T, m *Manager) {
	t.Helper()
	directory := filepath.Dir(m.path)
	backup := directory + "-offline"
	if err := os.Rename(directory, backup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(directory); err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
		if err := os.Rename(backup, directory); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(directory, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
}
