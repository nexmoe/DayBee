package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestVoiceprintMigrationAndPersistence(t *testing.T) {
	root, documents := t.TempDir(), t.TempDir()
	put(t, root, map[string]any{
		"nativeDevices": true,
		"settings": map[string]any{
			"desktop":   map[string]any{"language": "zh-CN", "startRecordingOnLaunch": false},
			"recording": map[string]any{"device": "native:usb", "recordSystemAudio": false},
		},
	})
	store, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	if settings.Desktop.VoiceprintSetup != protocol.VoiceprintPending {
		t.Fatal("legacy settings must offer optional setup")
	}
	if settings.Desktop.Language != protocol.LanguageChinese || settings.Desktop.StartRecordingOnLaunch || settings.Recording.Device != "native:usb" {
		t.Fatal("migration changed existing preferences")
	}
	for _, state := range []protocol.VoiceprintSetupState{protocol.VoiceprintSkipped, protocol.VoiceprintConfigured} {
		settings.Desktop.VoiceprintSetup = state
		if _, err := store.Save(settings); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(root, documents, nil, true)
		if err != nil || reopened.Settings() != settings {
			t.Fatalf("setup decision did not survive restart: %v", err)
		}
	}
}

func TestVoiceprintIgnoresRetiredDisabledSetting(t *testing.T) {
	settings := normalize([]byte(`{"desktop":{"voiceprintSetup":"ready"},"recording":{"voiceprintEnabled":false}}`), protocol.DefaultSettings(t.TempDir()))
	if settings.Desktop.VoiceprintSetup != protocol.VoiceprintConfigured {
		t.Fatal("retired switch reset the saved setup state")
	}
	raw, err := json.Marshal(settings)
	if err != nil || strings.Contains(string(raw), "voiceprintEnabled") {
		t.Fatal("retired switch survived settings migration", err)
	}
}

func TestInvalidVoiceprintSetupRemainsPending(t *testing.T) {
	settings := normalize([]byte(`{"desktop":{"voiceprintSetup":"unknown"}}`), protocol.DefaultSettings(t.TempDir()))
	if settings.Desktop.VoiceprintSetup != protocol.VoiceprintPending {
		t.Fatal("unknown setup state bypasses first-run choice")
	}
}
