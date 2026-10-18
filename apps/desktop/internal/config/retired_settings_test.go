package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestRetiredRecordingSettingsAreIgnoredAndRemovedOnSave(t *testing.T) {
	root, documents := t.TempDir(), t.TempDir()
	output := filepath.Join(documents, "custom-recordings")
	put(t, root, map[string]any{
		"nativeDevices": true,
		"settings": map[string]any{
			"desktop": map[string]any{
				"startRecordingOnLaunch": false, "language": "zh-CN", "voiceprintSetup": "skipped",
			},
			"recording": map[string]any{
				"audioFormat": "wav", "outputDir": output, "device": "native:usb",
			},
		},
	})
	store, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	if settings.Desktop.Language != protocol.LanguageChinese || settings.Desktop.VoiceprintSetup != protocol.VoiceprintSkipped || settings.Recording.OutputDir != output || settings.Recording.Device != "native:usb" {
		t.Fatal("retiring recording preferences changed unrelated settings", settings)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(filepath.Join(root, "desktop-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{encoded, persisted} {
		for _, retired := range []string{"startRecordingOnLaunch", "audioFormat"} {
			if strings.Contains(string(data), `"`+retired+`"`) {
				t.Fatalf("retired preference %s survived normalization: %s", retired, data)
			}
		}
	}
	reopened, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings() != settings {
		t.Fatal("normalized settings changed after reopening")
	}
}
