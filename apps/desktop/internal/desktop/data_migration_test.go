package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/assets"
	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/recorder"
)

func putLegacyState(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDayBeeImportsLegacySettingsStateAndModels(t *testing.T) {
	t.Setenv("DAYBEE_STORE_DIR", "")
	appData, documents := t.TempDir(), t.TempDir()
	service := &DesktopService{}
	paths := service.dataPaths(appData, documents)
	old := paths.legacy[0]
	oldStore, err := config.Open(old, documents, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := oldStore.Settings()
	settings.Recording.OutputDir = filepath.Join(documents, "RecBee")
	settings.Recording.VoiceprintAdaptation = true
	if _, err := oldStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	putLegacyState(t, filepath.Join(old, "voiceprint/profile.json"), `{"revision":9,"adaptationProgress":{"learnedClips":12,"assistedClips":3}}`)
	putLegacyState(t, filepath.Join(old, "microphone-activity/20261013.json"), "[]\n")
	putLegacyState(t, filepath.Join(old, "models/silero_vad.onnx"), "existing cached model")
	store, err := config.Open(paths.root, paths.documents, paths.legacy, false)
	if err != nil || store.Settings() != settings {
		t.Fatal("rename reset saved settings or output path", err)
	}
	if err := importLegacyState(paths.root, paths.legacy); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"voiceprint/profile.json", "microphone-activity/20261013.json", "desktop-settings.json"} {
		before, err := os.ReadFile(filepath.Join(old, relative))
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(paths.root, relative))
		if err != nil || string(before) != string(after) {
			t.Fatalf("legacy state changed or was lost: %s: %v", relative, err)
		}
	}
	models := assets.New(paths.root, paths.legacy, nil)
	if models.VADPath() != filepath.Join(old, "models/silero_vad.onnx") {
		t.Fatal("rename stopped reusing the existing model cache")
	}
	profile := filepath.Join(paths.root, "voiceprint/profile.json")
	if err := os.Remove(profile); err != nil {
		t.Fatal(err)
	}
	if err := importLegacyState(paths.root, paths.legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("second launch resurrected a cleared legacy voiceprint")
	}
}

func TestDayBeeMigrationKeepsExistingState(t *testing.T) {
	root, old := t.TempDir(), t.TempDir()
	relative := "voiceprint/profile.json"
	putLegacyState(t, filepath.Join(root, relative), "new profile")
	putLegacyState(t, filepath.Join(old, relative), "old profile")
	if err := importLegacyState(root, []string{old}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil || string(raw) != "new profile" {
		t.Fatal("rename overwrote the DayBee voiceprint", err)
	}
}

func TestDayBeeHistoryIncludesOldRecordingFolders(t *testing.T) {
	documents := t.TempDir()
	service := &DesktopService{defaultOutput: filepath.Join(documents, "DayBee"),
		legacyOutputs: []string{filepath.Join(documents, "RecBee"), filepath.Join(documents, "Eve Recorder")}}
	for _, path := range []string{
		filepath.Join(service.defaultOutput, "daybee_20261013_120000_microphone.wav"),
		filepath.Join(service.legacyOutputs[0], "eve_20261012_120000_microphone.wav"),
		filepath.Join(service.legacyOutputs[1], "eve_20261011_120000.wav"),
	} {
		putLegacyState(t, path, "saved audio")
	}
	if history := recorder.History(service.recordingRoots(t.TempDir())); len(history) != 3 {
		t.Fatalf("rename hid existing recordings: %d", len(history))
	}
}
