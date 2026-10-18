package config

import (
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestSystemAudioIsOptInAndPersists(t *testing.T) {
	root, documents := t.TempDir(), t.TempDir()
	store, err := Open(root, documents, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	if settings.Recording.RecordSystemAudio {
		t.Fatal("new or old settings must not opt in to system capture")
	}
	settings.Recording.RecordSystemAudio = true
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, documents, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings().Recording.RecordSystemAudio != protocol.SupportsSystemAudio {
		t.Fatal("system capture preference not persisted or unsupported on this platform")
	}
}
