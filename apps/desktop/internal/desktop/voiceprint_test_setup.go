package desktop

import (
	"errors"
	"sync"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// testVoiceprintSetup is the E2E stand-in for enrollment. It never touches a
// microphone, model, or voiceprint.Manager. newTestVoiceprintSetup is used only
// when DesktopService.test is set.
type testVoiceprintSetup struct {
	s       *DesktopService
	mu      sync.Mutex
	phase   protocol.VoiceprintPhase
	step    int
	speech  float64
	target  float64
	level   float64
	message string
	errText string
	timer   *time.Timer
	gen     int
	closed  bool
}

func newTestVoiceprintSetup(s *DesktopService) voiceprintSetup {
	return &testVoiceprintSetup{s: s, phase: protocol.VoiceprintIdle, target: voiceprintSampleSeconds}
}

func (v *testVoiceprintSetup) Snapshot() protocol.VoiceprintSnapshot {
	enrolled := false
	if v.s != nil && v.s.store != nil && v.s.store.Settings().Desktop.VoiceprintSetup == protocol.VoiceprintConfigured {
		enrolled = true
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	snap := protocol.VoiceprintSnapshot{
		Enrolled: enrolled, Phase: v.phase, Step: v.step,
		SpeechSeconds: v.speech, TargetSeconds: v.target, Level: v.level, Message: v.message,
	}
	if v.target > 0 && (v.phase == protocol.VoiceprintPreparing || v.phase == protocol.VoiceprintSampleReady || v.phase == protocol.VoiceprintProcessing || v.phase == protocol.VoiceprintReady) {
		progress := v.speech / v.target
		if progress > 1 {
			progress = 1
		}
		snap.Progress = &progress
	}
	if v.errText != "" {
		text := v.errText
		snap.Error = &text
	}
	return snap
}

func (v *testVoiceprintSetup) Active() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch v.phase {
	case protocol.VoiceprintPreparing, protocol.VoiceprintRecording, protocol.VoiceprintSampleReady, protocol.VoiceprintProcessing:
		return !v.closed
	default:
		return false
	}
}

func (v *testVoiceprintSetup) Start() error {
	if v.s == nil || !v.s.test {
		return errors.New("Voiceprint test setup is only available in test mode.")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return errors.New("Voiceprint setup is closed.")
	}
	switch v.phase {
	case protocol.VoiceprintPreparing, protocol.VoiceprintRecording, protocol.VoiceprintSampleReady, protocol.VoiceprintProcessing:
		return nil
	}
	v.gen++
	v.step = 0
	v.speech = 0
	v.level = 0
	v.errText = ""
	v.phase = protocol.VoiceprintPreparing
	v.target = voiceprintSampleSeconds
	v.message = "Preparing voiceprint sample."
	v.arm(v.gen)
	v.s.markDirty()
	return nil
}

func (v *testVoiceprintSetup) Advance() error {
	if v.s == nil || !v.s.test {
		return errors.New("Voiceprint test setup is only available in test mode.")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return errors.New("Voiceprint setup is closed.")
	}
	if v.phase != protocol.VoiceprintSampleReady || v.step >= voiceprintSamples {
		return errors.New("Voiceprint sample is not ready to continue.")
	}
	v.gen++
	v.step++
	v.speech = 0
	v.level = 0
	v.errText = ""
	v.phase = protocol.VoiceprintPreparing
	v.target = voiceprintSampleSeconds
	v.message = "Preparing voiceprint sample."
	if v.step == voiceprintSamples {
		v.target = voiceprintVerifySeconds
		v.message = "Preparing voiceprint check."
	}
	v.arm(v.gen)
	v.s.markDirty()
	return nil
}

func (v *testVoiceprintSetup) Cancel() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return
	}
	v.resetLocked()
	v.s.markDirty()
}

func (v *testVoiceprintSetup) Clear() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return errors.New("Voiceprint setup is closed.")
	}
	v.resetLocked()
	v.s.markDirty()
	return nil
}

func (v *testVoiceprintSetup) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
	v.gen++
	v.stopTimerLocked()
	v.phase = protocol.VoiceprintIdle
	v.step = 0
	v.speech = 0
	v.level = 0
	v.target = voiceprintSampleSeconds
	v.message = ""
	v.errText = ""
	if v.s != nil {
		v.s.markDirty()
	}
}

func (v *testVoiceprintSetup) resetLocked() {
	v.gen++
	v.stopTimerLocked()
	v.phase = protocol.VoiceprintIdle
	v.step = 0
	v.speech = 0
	v.level = 0
	v.target = voiceprintSampleSeconds
	v.message = ""
	v.errText = ""
}

func (v *testVoiceprintSetup) stopTimerLocked() {
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
}

func (v *testVoiceprintSetup) arm(gen int) {
	v.stopTimerLocked()
	v.timer = time.AfterFunc(100*time.Millisecond, func() { v.finish(gen) })
}

func (v *testVoiceprintSetup) finish(gen int) {
	v.mu.Lock()
	if v.closed || v.gen != gen || v.phase != protocol.VoiceprintPreparing {
		v.mu.Unlock()
		return
	}
	step := v.step
	v.speech = v.target
	v.level = 0.4
	if step < voiceprintSamples {
		v.phase = protocol.VoiceprintSampleReady
		v.message = "Sample ready."
		v.mu.Unlock()
		v.s.markDirty()
		return
	}
	v.phase = protocol.VoiceprintProcessing
	v.message = "Saving voiceprint."
	v.mu.Unlock()
	v.s.markDirty()
	err := v.persistReady()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.gen != gen {
		return
	}
	if err != nil {
		v.phase = protocol.VoiceprintError
		v.errText = err.Error()
		v.message = v.errText
	} else {
		v.phase = protocol.VoiceprintReady
		v.level = 0
		v.message = "Voiceprint is ready."
		v.errText = ""
	}
	v.s.markDirty()
}

// Enrolled follows the persisted setup state in this isolated test controller.
func (v *testVoiceprintSetup) persistReady() error {
	if v.s == nil || v.s.store == nil {
		return errors.New("Voiceprint settings are unavailable.")
	}
	settings := v.s.store.Settings()
	settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	if _, err := v.s.SaveSettings(settings); err != nil {
		return err
	}
	return nil
}
