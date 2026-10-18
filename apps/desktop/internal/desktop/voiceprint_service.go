package desktop

import (
	"errors"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type voiceprintSetup interface {
	Snapshot() protocol.VoiceprintSnapshot
	Start() error
	Advance() error
	Cancel()
	Clear() error
	Close()
	Active() bool
}

// Imported profiles and older settings can disagree about first-run completion.
func (s *DesktopService) reconcileVoiceprintSetup(enrolled bool) error {
	s.saves.Lock()
	defer s.saves.Unlock()
	settings := s.store.Settings()
	switch {
	case enrolled && settings.Desktop.VoiceprintSetup == protocol.VoiceprintPending:
		settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	case !enrolled && settings.Desktop.VoiceprintSetup == protocol.VoiceprintConfigured:
		settings.Desktop.VoiceprintSetup = protocol.VoiceprintPending
	default:
		return nil
	}
	_, err := s.store.Save(settings)
	if err == nil {
		s.markDirty()
	}
	return err
}

func (s *DesktopService) StartVoiceprintEnrollment() (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	s.captureOps.Lock()
	defer s.captureOps.Unlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup == nil {
		return s.snapshot(), errors.New("Voiceprint initialization is unavailable.")
	}
	permission := s.RequestMicrophonePermission()
	if permission.Supported && permission.State != protocol.PermissionAuthorized {
		return s.snapshot(), errors.New(permission.Message)
	}
	err := s.voiceSetup.Start()
	s.markDirty()
	return s.snapshot(), err
}

func (s *DesktopService) AdvanceVoiceprintEnrollment() (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup == nil {
		return s.snapshot(), errors.New("Voiceprint initialization is unavailable.")
	}
	err := s.voiceSetup.Advance()
	s.markDirty()
	return s.snapshot(), err
}

func (s *DesktopService) CancelVoiceprintEnrollment() (protocol.DesktopSnapshot, error) {
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil {
		s.voiceSetup.Cancel()
	}
	s.markDirty()
	return s.snapshot(), nil
}

func (s *DesktopService) SkipVoiceprintSetup() (protocol.DesktopSnapshot, error) {
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil {
		s.voiceSetup.Cancel()
	}
	settings := s.store.Settings()
	wasPending := settings.Desktop.VoiceprintSetup == protocol.VoiceprintPending
	if s.voiceSetup == nil || !s.voiceSetup.Snapshot().Enrolled {
		settings.Desktop.VoiceprintSetup = protocol.VoiceprintSkipped
	} else {
		settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	}
	_, err := s.SaveSettings(settings)
	shouldStart := s.voiceStartRequested.Swap(false) || (settings.Desktop.StartRecordingOnLaunch && !s.test)
	if err == nil && wasPending && shouldStart {
		_, err = s.StartRecording()
	}
	s.markDirty()
	return s.snapshot(), err
}

func (s *DesktopService) ClearVoiceprint() (protocol.DesktopSnapshot, error) {
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup == nil {
		return s.snapshot(), errors.New("Voiceprint initialization is unavailable.")
	}
	if err := s.voiceSetup.Clear(); err != nil {
		return s.snapshot(), err
	}
	s.refreshVoiceprintReference()
	settings := s.store.Settings()
	settings.Desktop.VoiceprintSetup = protocol.VoiceprintSkipped
	_, err := s.SaveSettings(settings)
	s.markDirty()
	return s.snapshot(), err
}

func (s *DesktopService) refreshVoiceprintReference() {
	resetter, ok := s.engine.(interface{ SetVoiceprint(*voiceprint.Manager) })
	if !ok {
		return
	}
	resetter.SetVoiceprint(s.voiceprints)
}
