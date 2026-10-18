package desktop

import (
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestSkipVoiceprintSetupWithExistingProfileCompletesSetup(t *testing.T) {
	s, manager, root := adaptationService(t)
	s.voiceSetup = newVoiceprintEnrollment(s, nil, manager)
	t.Cleanup(s.voiceSetup.Close)
	t.Cleanup(manager.Close)
	t.Cleanup(func() { _ = s.engine.Close() })
	before := manager.Snapshot().Revision
	if s.store.Settings().Desktop.VoiceprintSetup != protocol.VoiceprintPending || !s.voiceSetup.Snapshot().Enrolled {
		t.Fatal("expected a saved voiceprint with stale first-run settings")
	}
	if _, err := s.SkipVoiceprintSetup(); err != nil {
		t.Fatal(err)
	}
	if s.store.Settings().Desktop.VoiceprintSetup != protocol.VoiceprintConfigured {
		t.Fatal("saved voiceprint still blocks recording behind first-run setup")
	}
	if !manager.Enrolled() || manager.Snapshot().Revision != before {
		t.Fatal("completing setup changed the saved voiceprint")
	}
	reopened, err := config.Open(root, t.TempDir(), nil, false)
	if err != nil || reopened.Settings().Desktop.VoiceprintSetup != protocol.VoiceprintConfigured {
		t.Fatalf("completed setup did not survive restart: %v", err)
	}
}

func TestStartRecordingWithExistingProfileCompletesPendingSetup(t *testing.T) {
	s, manager, _ := adaptationService(t)
	s.voiceSetup = newVoiceprintEnrollment(s, nil, manager)
	s.permission = protocol.MicrophonePermissionStatus{State: protocol.PermissionAuthorized, Supported: true}
	t.Cleanup(s.voiceSetup.Close)
	t.Cleanup(manager.Close)
	t.Cleanup(func() { _ = s.engine.Close() })
	if _, err := s.StartRecording(); err != nil {
		t.Fatal(err)
	}
	if !s.engine.Status().Recording || s.store.Settings().Desktop.VoiceprintSetup != protocol.VoiceprintConfigured {
		t.Fatal("existing voiceprint did not allow the requested recording to start")
	}
	if s.voiceStartRequested.Load() {
		t.Fatal("recording was redirected to first-run setup")
	}
}

func TestReconcileVoiceprintSetupMatchesSavedProfile(t *testing.T) {
	for _, test := range []struct {
		state    protocol.VoiceprintSetupState
		enrolled bool
		want     protocol.VoiceprintSetupState
	}{
		{protocol.VoiceprintPending, true, protocol.VoiceprintConfigured},
		{protocol.VoiceprintPending, false, protocol.VoiceprintPending},
		{protocol.VoiceprintConfigured, true, protocol.VoiceprintConfigured},
		{protocol.VoiceprintConfigured, false, protocol.VoiceprintPending},
		{protocol.VoiceprintSkipped, true, protocol.VoiceprintSkipped},
		{protocol.VoiceprintSkipped, false, protocol.VoiceprintSkipped},
	} {
		t.Run(string(test.state)+"/enrolled="+map[bool]string{true: "true", false: "false"}[test.enrolled], func(t *testing.T) {
			s, manager, root := adaptationService(t)
			t.Cleanup(manager.Close)
			t.Cleanup(func() { _ = s.engine.Close() })
			settings := s.store.Settings()
			settings.Desktop.VoiceprintSetup = test.state
			if _, err := s.store.Save(settings); err != nil {
				t.Fatal(err)
			}
			if err := s.reconcileVoiceprintSetup(test.enrolled); err != nil {
				t.Fatal(err)
			}
			settings.Desktop.VoiceprintSetup = test.want
			if s.store.Settings() != settings {
				t.Fatal("setup state did not match the saved profile or changed unrelated settings")
			}
			reopened, err := config.Open(root, t.TempDir(), nil, false)
			if err != nil || reopened.Settings() != settings {
				t.Fatalf("reconciled settings did not survive restart: %v", err)
			}
		})
	}
}
