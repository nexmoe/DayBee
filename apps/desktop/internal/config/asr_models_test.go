package config

import (
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestModelSelectionPersistsAndMissingSettingsUsePlatformDefault(t *testing.T) {
	defaults := protocol.DefaultSettings(t.TempDir())
	if defaults.Recording.ASRModel != protocol.DefaultASRModel {
		t.Fatal("new installations must use the platform default")
	}
	for _, raw := range []string{`{}`, `{"recording":{}}`, `{"recording":{"asrModel":"unknown"}}`, `{"recording":{"asrModel":""}}`, `{"recording":{"asrModel":null}}`} {
		if got := normalize([]byte(raw), defaults).Recording.ASRModel; got != protocol.DefaultASRModel {
			t.Fatal("old/invalid setting lost default", got)
		}
	}
	root, documents := t.TempDir(), t.TempDir()
	store, err := Open(root, documents, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if store.Settings().Recording.ASRModel != protocol.DefaultASRModel {
		t.Fatal("initial store did not select the platform default")
	}
	for _, model := range []protocol.ASRModel{protocol.DefaultASRModel, protocol.ASRWhisperTiny, protocol.ASRWhisperBase, protocol.ASRWhisperSmall, protocol.ASRFireRedAED, protocol.ASRFireRedCTC, protocol.ASRSenseVoice, protocol.ASRQwen3} {
		settings := store.Settings()
		settings.Recording.ASRModel = model
		if _, err := store.Save(settings); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(root, documents, nil, true)
		if err != nil || reopened.Settings().Recording.ASRModel != model {
			t.Fatal("model selection not retained", model, err)
		}
	}
}

func TestLegacyExplicitModelSelectionIsPreserved(t *testing.T) {
	for _, model := range []protocol.ASRModel{protocol.ASRQwen3, protocol.ASRFireRedAED, protocol.ASRFireRedCTC, protocol.ASRSenseVoice} {
		t.Run(string(model), func(t *testing.T) {
			root, legacy, documents := t.TempDir(), t.TempDir(), t.TempDir()
			put(t, legacy, map[string]any{"settings": map[string]any{"recording": map[string]any{"asrModel": model}}})
			store, err := Open(root, documents, []string{legacy}, true)
			if err != nil || store.Settings().Recording.ASRModel != model {
				t.Fatal("migration replaced an explicit model selection", model, err)
			}
		})
	}
}
