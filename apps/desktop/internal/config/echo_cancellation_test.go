package config

import (
	"path/filepath"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestExistingSettingsGetEchoCancellationWithoutResettingPreferences(t *testing.T) {
	root, documents := t.TempDir(), t.TempDir()
	put(t, root, map[string]any{"nativeDevices": true, "settings": map[string]any{
		"recording": map[string]any{"device": "native:my-microphone", "autoSwitchDevice": false},
	}})
	store, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	if settings.Recording.EchoCancellation != protocol.SupportsEchoCancellation ||
		settings.Recording.Device != "native:my-microphone" || settings.Recording.AutoSwitchDevice ||
		settings.Recording.OutputDir != filepath.Join(documents, "DayBee") {
		t.Fatalf("unexpected migrated settings: %+v", settings.Recording)
	}
	settings.Recording.EchoCancellation = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, documents, nil, true)
	if err != nil || reopened.Settings().Recording.EchoCancellation {
		t.Fatal("explicit raw recording preference was not preserved", err)
	}
}
