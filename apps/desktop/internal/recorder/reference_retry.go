package recorder

import (
	"errors"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// RetryVoiceprintReference reopens capture after authorization or a reference
// failure. Drain queued audio and keep the current file, VAD offsets and clock.
// A fresh reference epoch prevents old callbacks from undoing the retry.
func (e *Engine) RetryVoiceprintReference() error {
	e.op.Lock()
	defer e.op.Unlock()
	return e.call(func() error {
		if e.vad == nil || !e.Status().Recording || !e.wantsSystemReference() {
			return errors.New("Start recording with voiceprint recognition before retrying the playback check.")
		}
		if e.Status().VoiceprintReference == protocol.VoiceprintReferenceAvailable {
			return nil
		}
		if err := e.restartCaptureOnly(e.Settings().Recording.Device); err != nil {
			e.safeStop(err)
			return err
		}
		return nil
	})
}
