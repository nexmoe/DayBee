package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/DayBee/apps/desktop/internal/assets"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestPrepareASRModelWhileRecordingDoesNotChangeSelection(t *testing.T) {
	root := t.TempDir()
	store, err := config.Open(root, t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := asrmodel.Find(protocol.ASRFireRedCTC)
	for _, name := range model.Files {
		path := filepath.Join(root, "models", model.Directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("cached"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &DesktopService{ctx: context.Background(), ready: make(chan struct{}), store: store,
		models: assets.New(root, nil, nil), test: true, settingsTheme: func(protocol.AppSettings) {}}
	s.engine = newTestEngine(store.Settings(), func() {})
	close(s.ready)
	if err := s.engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.engine.Close()
	snapshot, err := s.PrepareASRModel(model.ID)
	if err != nil {
		t.Fatal("preparing a model must not require stopping recording", err)
	}
	if !snapshot.Status.Recording || snapshot.Settings.Recording.ASRModel != protocol.DefaultASRModel {
		t.Fatal("preparation interrupted recording or switched the active model")
	}
	next := store.Settings()
	next.Recording.ASRModel = model.ID
	if _, err := s.SaveSettings(next); err != nil {
		t.Fatal("activation during recording failed", err)
	}
	if store.Settings().Recording.ASRModel != model.ID || !s.engine.Status().Recording {
		t.Fatal("activation lost the selection or interrupted recording")
	}
}
