package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func TestLoadTestVoiceprintLeavesMissingProfileNil(t *testing.T) {
	service := &DesktopService{}
	if err := service.loadTestVoiceprint(t.TempDir()); err != nil || service.voiceprints != nil {
		t.Fatalf("err %v loaded %v", err, service.voiceprints != nil)
	}
}

func TestLoadTestVoiceprintLoadsExistingProfileAndSyncs(t *testing.T) {
	root := t.TempDir()
	writer, err := voiceprint.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveProfile(adaptationProfile(2)); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	store, err := config.Open(root, t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.Recording.VoiceprintAdaptation = true
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	service := &DesktopService{store: store}
	if err := service.loadTestVoiceprint(root); err != nil || service.voiceprints == nil {
		t.Fatalf("err %v loaded %v", err, service.voiceprints != nil)
	}
	t.Cleanup(service.voiceprints.Close)
	if count, _ := service.voiceprints.RuntimeCounts(); count != 1 {
		t.Fatalf("count %d", count)
	}
	revision := service.voiceprints.Snapshot().Revision
	service.voiceprints.SetAdaptationEnabled(true)
	if service.voiceprints.Snapshot().Revision != revision {
		t.Fatal("loaded manager did not follow the stored on switch")
	}
}

func TestLoadTestVoiceprintKeepsStoredSwitchOff(t *testing.T) {
	root := t.TempDir()
	writer, err := voiceprint.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveProfile(adaptationProfile(0)); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	store, err := config.Open(root, t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if store.Settings().Recording.VoiceprintAdaptation {
		t.Fatal("store defaulted adaptation on")
	}
	service := &DesktopService{store: store}
	if err := service.loadTestVoiceprint(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.voiceprints.Close)
	revision := service.voiceprints.Snapshot().Revision
	service.voiceprints.SetAdaptationEnabled(false)
	if service.voiceprints.Snapshot().Revision != revision {
		t.Fatal("loaded manager turned adaptation on")
	}
}

func TestLoadTestVoiceprintReturnsInvalidProfile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "voiceprint"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "voiceprint", "profile.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	service := &DesktopService{}
	err := service.loadTestVoiceprint(root)
	if err == nil || service.voiceprints == nil {
		t.Fatalf("err %v loaded %v", err, service.voiceprints != nil)
	}
	service.voiceprints.Close()
}
