package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

func TestVoiceprintAdaptationDefaultsOffAndFollowsStoredSetting(t *testing.T) {
	raw, err := json.Marshal(protocol.DefaultSettings("recordings"))
	if err != nil || !strings.Contains(string(raw), `"voiceprintAdaptation":false`) {
		t.Fatalf("default settings %s: %v", raw, err)
	}
	s, manager, _ := adaptationService(t)
	if s.store.Settings().Recording.VoiceprintAdaptation {
		t.Fatal("stored adaptation defaulted on")
	}
	revision := manager.Snapshot().Revision
	s.syncVoiceprintAdaptation()
	if manager.Snapshot().Revision != revision {
		t.Fatal("startup sync changed the default-off switch")
	}
	enabled := s.store.Settings()
	enabled.Recording.VoiceprintAdaptation = true
	if _, err := s.SaveSettings(enabled); err != nil || !s.store.Settings().Recording.VoiceprintAdaptation {
		t.Fatal(err)
	}
	on := manager.Snapshot().Revision
	if on == revision {
		t.Fatal("successful save left the manager switch off")
	}
	manager.SetAdaptationEnabled(true)
	if manager.Snapshot().Revision != on {
		t.Fatal("manager switch did not stay on with the stored setting")
	}
	enabled.Recording.VoiceprintAdaptation = false
	if _, err := s.SaveSettings(enabled); err != nil || s.store.Settings().Recording.VoiceprintAdaptation {
		t.Fatal(err)
	}
	off := manager.Snapshot().Revision
	if off == on {
		t.Fatal("turning adaptation off did not reach the manager")
	}
	manager.SetAdaptationEnabled(false)
	if manager.Snapshot().Revision != off {
		t.Fatal("manager switch did not stay off with the stored setting")
	}
}

func TestVoiceprintAdaptationFollowsStoreWhenApplyRestoreFails(t *testing.T) {
	s, manager, root := adaptationService(t)
	before := manager.Snapshot().Revision
	s.engine = applyFailEngine{engine: s.engine, root: root}
	next := s.store.Settings()
	next.Recording.VoiceprintAdaptation = true
	returned, err := s.SaveSettings(next)
	if err == nil || !strings.Contains(err.Error(), "apply failed") {
		t.Fatal(err)
	}
	if returned.Recording.VoiceprintAdaptation || !s.store.Settings().Recording.VoiceprintAdaptation {
		t.Fatalf("returned %#v store %#v", returned.Recording.VoiceprintAdaptation, s.store.Settings().Recording.VoiceprintAdaptation)
	}
	on := manager.Snapshot().Revision
	if on == before {
		t.Fatal("manager followed the returned settings instead of the stored switch")
	}
	manager.SetAdaptationEnabled(true)
	if manager.Snapshot().Revision != on || !s.store.Settings().Recording.VoiceprintAdaptation {
		t.Fatal("manager switch did not match the stored on setting")
	}
}

func TestClearVoiceprintAdaptationKeepsEnrollment(t *testing.T) {
	s, manager, root := adaptationService(t)
	if err := manager.SaveProfile(adaptationProfile(2)); err != nil {
		t.Fatal(err)
	}
	if count, _ := manager.RuntimeCounts(); count != 1 {
		t.Fatalf("count %d", count)
	}
	reloaded, err := voiceprint.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := reloaded.RuntimeCounts(); count != 1 || reloaded.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("adaptation profile did not persist")
	}
	s.engine.Start()
	if _, err := s.ClearVoiceprintAdaptation(); err != nil {
		t.Fatal(err)
	}
	if count, _ := manager.RuntimeCounts(); count != 0 {
		t.Fatalf("count %d", count)
	}
	if got := manager.Snapshot(); len(got.Adaptation) != 0 || got.Embeddings[0][0] != 1 || got.Embeddings[0][1] != 2 {
		t.Fatalf("clear changed enrollment %#v", got.Embeddings)
	}
	persisted, err := voiceprint.New(root, nil)
	if err != nil || len(persisted.Snapshot().Adaptation) != 0 || persisted.Snapshot().Embeddings[0][0] != 1 {
		t.Fatalf("reloaded %#v %v", persisted.Snapshot(), err)
	}
}

func TestClearVoiceprintAdaptationRejectsActiveEnrollment(t *testing.T) {
	s, manager, _ := adaptationService(t)
	if err := manager.SaveProfile(adaptationProfile(2)); err != nil {
		t.Fatal(err)
	}
	s.voiceSetup = newTestVoiceprintSetup(s)
	if err := s.voiceSetup.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.voiceSetup.Cancel)
	if _, err := s.ClearVoiceprintAdaptation(); err == nil || !strings.Contains(err.Error(), "voiceprint enrollment") {
		t.Fatal(err)
	}
	if count, _ := manager.RuntimeCounts(); count != 1 || manager.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("enrollment rejection cleared the profile")
	}
}

func TestClearVoiceprintRefreshesReferenceWhenSettingsSaveFails(t *testing.T) {
	s, manager, root := adaptationService(t)
	resetter := &referenceResetter{engine: s.engine}
	s.engine = resetter
	s.voiceSetup = newTestVoiceprintSetup(s)
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearVoiceprint(); err == nil {
		t.Fatal("settings save should fail")
	}
	if resetter.calls != 1 || resetter.got != manager {
		t.Fatalf("refresh calls %d same manager %v", resetter.calls, resetter.got == manager)
	}
	if s.store.Settings().Desktop.VoiceprintSetup == protocol.VoiceprintSkipped {
		t.Fatal("failed save persisted the cleared setup")
	}
}

type referenceResetter struct {
	engine
	calls int
	got   *voiceprint.Manager
}

func (r *referenceResetter) SetVoiceprint(manager *voiceprint.Manager) {
	r.calls++
	r.got = manager
}

func TestVoiceprintSnapshotReadsAdaptationCountWithoutToggling(t *testing.T) {
	s, manager, _ := adaptationService(t)
	if err := manager.SaveProfile(adaptationProfile(2)); err != nil {
		t.Fatal(err)
	}
	s.voiceSetup = newTestVoiceprintSetup(s)
	settings := s.store.Settings()
	settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	if _, err := s.store.Save(settings); err != nil {
		t.Fatal(err)
	}
	revision := manager.Snapshot().Revision
	first := s.voiceprintSnapshot()
	second := s.voiceprintSnapshot()
	if !first.Enrolled || first.AdaptationCount != 1 || second.AdaptationCount != 1 || manager.Snapshot().Revision != revision {
		t.Fatalf("enrolled %v count %d/%d revision changed %v", first.Enrolled, first.AdaptationCount, second.AdaptationCount, manager.Snapshot().Revision != revision)
	}
}

type applyFailEngine struct {
	engine
	root string
}

func (e applyFailEngine) Apply(protocol.AppSettings) error {
	if err := os.Chmod(e.root, 0555); err != nil {
		return err
	}
	return errors.New("apply failed")
}

func adaptationService(t *testing.T) (*DesktopService, *voiceprint.Manager, string) {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	store, err := config.Open(root, t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := voiceprint.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveProfile(adaptationProfile(0)); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	close(ready)
	settings := store.Settings()
	fixture := protocol.DesktopSnapshot{}
	service := &DesktopService{
		ready: ready, store: store, engine: newTestEngine(settings, func() {}), test: true, fixture: &fixture,
		settingsTheme: func(protocol.AppSettings) {}, voiceprints: manager,
	}
	service.syncVoiceprintAdaptation()
	return service, manager, root
}

func adaptationProfile(mark float32) *voiceprint.Profile {
	vector := make([]float32, 192)
	vector[0] = 1
	vector[1] = mark
	profile := &voiceprint.Profile{Version: 2, Model: voiceprint.ModelName, Embeddings: [][]float32{vector}}
	if mark != 0 {
		learned := make([]float32, 192)
		learned[0] = 1
		learned[1] = mark
		profile.Adaptation = [][]float32{learned}
	}
	return profile
}
