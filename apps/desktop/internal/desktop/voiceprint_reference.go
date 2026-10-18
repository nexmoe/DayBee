package desktop

import (
	"errors"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func (s *DesktopService) OpenSystemAudioSettings() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return mygo.Shell.OpenExternal(audio.SystemAudioSettingsURL()) == nil
}

func (s *DesktopService) RetryVoiceprintReference() (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	s.captureOps.Lock()
	defer s.captureOps.Unlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		return s.snapshot(), errors.New("Finish or cancel voiceprint enrollment before retrying the playback check.")
	}
	retry, ok := s.engine.(interface{ RetryVoiceprintReference() error })
	if !ok {
		return s.snapshot(), errors.New("The playback check cannot be retried on this platform.")
	}
	err := retry.RetryVoiceprintReference()
	s.markDirty()
	return s.snapshot(), err
}
