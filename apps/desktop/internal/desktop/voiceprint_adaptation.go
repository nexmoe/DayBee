package desktop

import (
	"errors"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func (s *DesktopService) syncVoiceprintAdaptation() {
	if s == nil || s.store == nil || s.voiceprints == nil {
		return
	}
	s.voiceprints.SetAdaptationEnabled(s.store.Settings().Recording.VoiceprintAdaptation)
}

func (s *DesktopService) voiceprintSnapshot() protocol.VoiceprintSnapshot {
	snap := protocol.DefaultVoiceprint()
	if s.voiceSetup != nil {
		snap = s.voiceSetup.Snapshot()
	}
	if s.voiceprints != nil {
		count, progress := s.voiceprints.LearningProgress()
		snap.AdaptationCount = count
		if progress != nil {
			snap.Adaptation = &protocol.VoiceprintAdaptationProgress{LearnedClips: progress.LearnedClips, AssistedClips: progress.AssistedClips}
		}
	}
	return snap
}

func (s *DesktopService) ClearVoiceprintAdaptation() (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	s.captureOps.Lock()
	defer s.captureOps.Unlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		return s.snapshot(), errors.New("Finish or cancel voiceprint enrollment before clearing learned samples.")
	}
	if s.voiceprints == nil {
		return s.snapshot(), errors.New("Voiceprint initialization is unavailable.")
	}
	if err := s.voiceprints.ClearAdaptation(); err != nil {
		return s.snapshot(), err
	}
	s.markDirty()
	return s.snapshot(), nil
}
