package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func put(t *testing.T, root string, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "desktop-settings.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestImportPreservesDataAndIsIdempotent(t *testing.T) {
	root, old, documents := t.TempDir(), t.TempDir(), t.TempDir()
	settings := protocol.DefaultSettings(filepath.Join(documents, "original recordings"))
	settings.Desktop.Language = protocol.LanguageChinese
	settings.Desktop.Theme = protocol.ThemeDark
	settings.Recording.Device = "browser-hashed-device"
	settings.Recording.DisableASR = true
	raw := put(t, old, map[string]any{"settings": settings, "windowState": WindowState{700, 500}})
	store, err := Open(root, documents, []string{old}, true)
	if err != nil {
		t.Fatal(err)
	}
	settings.Recording.Device = "default"
	if store.Settings() != settings || store.Window() != (WindowState{700, 500}) {
		t.Fatalf("migration changed fields: %+v", store.Settings())
	}
	backup, err := os.ReadFile(filepath.Join(root, "desktop-settings.electron-backup.json"))
	if err != nil || string(backup) != string(raw) {
		t.Fatal("missing exact source backup", err)
	}
	native := settings
	native.Recording.Device = "native:usb"
	if _, err = store.Save(native); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, documents, []string{old}, true)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings() != native {
		t.Fatal("second launch reset native settings")
	}
	oldRaw, _ := os.ReadFile(filepath.Join(old, "desktop-settings.json"))
	if string(oldRaw) != string(raw) {
		t.Fatal("legacy data was modified")
	}
}
func TestCanonicalPrecedenceAndCorruptFallback(t *testing.T) {
	root, old := t.TempDir(), t.TempDir()
	canonical := protocol.DefaultSettings(t.TempDir())
	canonical.Desktop.LaunchAtLogin = false
	put(t, root, map[string]any{"settings": canonical, "nativeDevices": true})
	put(t, old, map[string]any{"settings": protocol.DefaultSettings(t.TempDir())})
	store, err := Open(root, t.TempDir(), []string{old}, true)
	if err != nil || store.Settings() != canonical {
		t.Fatal("valid canonical settings not preferred", err)
	}
	if err = os.WriteFile(filepath.Join(root, "desktop-settings.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(root, t.TempDir(), []string{old}, true); err != nil {
		t.Fatal("valid legacy fallback failed", err)
	}
	backup, _ := os.ReadFile(filepath.Join(root, "desktop-settings.corrupt-backup.json"))
	if string(backup) != "broken" {
		t.Fatal("corrupt canonical was not backed up")
	}
}
func TestDefaultsIsolationAndValidation(t *testing.T) {
	root, documents := t.TempDir(), t.TempDir()
	put(t, root, map[string]any{"settings": map[string]any{"desktop": map[string]any{"language": "invalid", "launchAtLogin": "false"}, "recording": map[string]any{"segmentMinutes": -1, "outputDir": "relative", "autoSwitchConfirmations": 0}}})
	store, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if store.Settings() != protocol.DefaultSettings(filepath.Join(documents, "DayBee")) {
		t.Fatalf("bad defaults: %+v", store.Settings())
	}
	if _, err = Open(t.TempDir(), documents, nil, true); err != nil {
		t.Fatal(err)
	}
}

func TestNonObjectConfigurationFallsBackWithoutLosingSource(t *testing.T) {
	for _, invalid := range []string{`{"settings":42}`, `{"settings":[]}`, `{"settings":null}`} {
		t.Run(invalid, func(t *testing.T) {
			root, old := t.TempDir(), t.TempDir()
			path := filepath.Join(root, "desktop-settings.json")
			if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
				t.Fatal(err)
			}
			want := protocol.DefaultSettings(t.TempDir())
			put(t, old, map[string]any{"settings": want})
			store, err := Open(root, t.TempDir(), []string{old}, true)
			if err != nil || store.Settings() != want {
				t.Fatal("invalid canonical prevented legacy import", err)
			}
			raw, _ := os.ReadFile(filepath.Join(root, "desktop-settings.corrupt-backup.json"))
			if string(raw) != invalid {
				t.Fatal("invalid source was not backed up")
			}
		})
	}
}
func TestAtomicReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := WriteJSON(path, map[string]string{"text": "旧"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, map[string]string{"text": "新"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &value) != nil || value["text"] != "新" {
		t.Fatal("replacement is incomplete")
	}
}
