package desktop

import (
	"path/filepath"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestDefaultDataPathsReuseProductionSettings(t *testing.T) {
	t.Setenv("DAYBEE_STORE_DIR", "")
	appData, documents := t.TempDir(), t.TempDir()
	production := filepath.Join(appData, "DayBee")
	store, err := config.Open(production, documents, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.Recording.OutputDir = filepath.Join(documents, "saved-recordings")
	settings.Recording.ASRModel = protocol.ASRFireRedCTC
	settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	s := &DesktopService{}
	paths := s.dataPaths(appData, documents)
	if s.isolated() || paths.root != production || paths.documents != documents || len(paths.legacy) != 3 || paths.legacy[0] != filepath.Join(appData, "Eve Recorder") {
		t.Fatalf("default run did not use production data: %+v", paths)
	}
	reopened, err := config.Open(paths.root, paths.documents, paths.legacy, false)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings() != settings {
		t.Fatal("default run replaced saved output, model or voiceprint settings")
	}
}

func TestDataPathIsolationForOverridesAndTests(t *testing.T) {
	for _, test := range []struct {
		name     string
		testMode bool
		override bool
	}{
		{"explicit directory", false, true},
		{"test fallback", true, false},
		{"test explicit directory", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			appData, documents := t.TempDir(), t.TempDir()
			root := ""
			want := filepath.Join(appData, "DayBee Development")
			if test.override {
				root = t.TempDir()
				want = root
			}
			t.Setenv("DAYBEE_STORE_DIR", root)
			s := &DesktopService{test: test.testMode}
			paths := s.dataPaths(appData, documents)
			if !s.isolated() || paths.root != want || paths.documents != want || len(paths.legacy) != 0 || s.shouldApplyLoginItem() {
				t.Fatalf("isolated run could access production data or login items: %+v", paths)
			}
		})
	}
}
