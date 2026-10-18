package recorder

import (
	"log/slog"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func (e *Engine) wantsSystemReference() bool {
	return e.referenceWanted(e.Settings())
}

func (e *Engine) referenceWanted(settings protocol.AppSettings) bool {
	if settings.Recording.DisableASR {
		return false
	}
	e.mu.RLock()
	matcher := e.voice
	e.mu.RUnlock()
	return matcher != nil && matcher.Enrolled()
}

const referenceClockSlack = 150 * time.Millisecond

func (e *Engine) captureOptions(id string) audio.CaptureOptions {
	want := e.wantsSystemReference()
	e.resetEcho()
	e.refClock = time.Time{}
	epoch := e.refEpoch.Add(1)
	phase := protocol.VoiceprintReferenceOff
	var active uint64
	if want {
		active = epoch
		phase = protocol.VoiceprintReferenceStarting
	}
	e.refActive.Store(active)
	e.storePhase(phase)
	settings := e.Settings()
	return audio.CaptureOptions{
		DeviceID: id, EchoCancellation: settings.Recording.EchoCancellation,
		SystemAudio: settings.Recording.RecordSystemAudio, SystemReference: want,
		OnReferenceUnavailable: func(err error) { e.noteReferenceFailure(epoch, err) },
	}
}

func (e *Engine) noteReferenceFailure(epoch uint64, err error) {
	if e.refEpoch.Load() != epoch {
		return
	}
	e.refFailedEpoch.Store(epoch)
	e.refPhase.Store(protocol.VoiceprintReferenceUnavailable)
	message := "System playback check failed."
	if err != nil {
		message = err.Error()
	}
	slog.Warn("Voiceprint playback reference unavailable; adaptation paused", "error", message)
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.VoiceprintReference = protocol.VoiceprintReferenceUnavailable
		s.VoiceprintReferenceError = message
	})
}

// settleReferenceFailure runs on the event loop. The callback only records the epoch.
func (e *Engine) settleReferenceFailure() {
	failed := e.refFailedEpoch.Load()
	if failed == 0 || failed != e.refEpoch.Load() {
		return
	}
	e.resetEcho()
	e.refClock = time.Time{}
	e.refActive.Store(0)
}

func (e *Engine) noteReferenceAudio() {
	if e.refActive.Load() == 0 {
		return
	}
	phase, _ := e.refPhase.Load().(string)
	if phase == protocol.VoiceprintReferenceStarting {
		e.storePhase(protocol.VoiceprintReferenceAvailable)
	}
}

// feedSystemReference places reference-only system audio on an arrival-time clock.
// Native capture does not forward a hardware timestamp. CapturedAt is the queue
// time, and a stall beyond 150ms leaves an uncovered hole instead of a continuous span.
func (e *Engine) feedSystemReference(packet audio.Packet) {
	if e.refActive.Load() == 0 || len(packet.Samples) == 0 || e.echoRef == nil {
		return
	}
	received := packet.CapturedAt
	if received.IsZero() {
		received = e.referenceNow()
	}
	if !e.refClock.IsZero() {
		delta := received.Sub(e.refClock)
		if delta > referenceClockSlack || delta < -referenceClockSlack {
			e.refClock = received
		}
	} else {
		e.refClock = received
	}
	e.echoRef.AppendAt(e.refClock, packet.Samples)
	e.refClock = echoAt(e.refClock, len(packet.Samples))
	e.noteReferenceAudio()
}

func (e *Engine) referenceNow() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

func (e *Engine) retireReference() {
	if e == nil {
		return
	}
	e.resetEcho()
	e.refClock = time.Time{}
	e.refEpoch.Add(1)
	e.refActive.Store(0)
	e.storePhase(protocol.VoiceprintReferenceOff)
}

func (e *Engine) storePhase(phase string) {
	e.refPhase.Store(phase)
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.VoiceprintReference = phase
		s.VoiceprintReferenceError = ""
	})
}

func (e *Engine) restartCaptureOnly(id string) error {
	if err := e.capture.Stop(); err != nil {
		return err
	}
	for {
		select {
		case packet := <-e.packets:
			if err := e.consume(packet); err != nil {
				return err
			}
		default:
			return e.startCapture(id, false)
		}
	}
}

func (e *Engine) syncReferenceCapture() error {
	if e.vad == nil {
		if !e.wantsSystemReference() {
			e.retireReference()
		}
		e.publishMismatch(e.activeDeviceID())
		return nil
	}
	if (e.refActive.Load() != 0) == e.wantsSystemReference() {
		e.publishMismatch(e.activeDeviceID())
		return nil
	}
	return e.restartCaptureOnly(e.Settings().Recording.Device)
}

func (e *Engine) activeDeviceID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.activeDevice
}

func (e *Engine) noteActiveDevice(id string) {
	e.mu.Lock()
	changed := e.activeDevice != id
	e.activeDevice = id
	e.mu.Unlock()
	if changed && realDevice(id) {
		e.publishMismatch(id)
	}
	e.noteVoiceContext(id)
}

func (e *Engine) publishMismatch(device string) {
	e.mu.RLock()
	matcher := e.voice
	echo := e.settings.Recording.EchoCancellation
	e.mu.RUnlock()
	mismatch := false
	if realDevice(device) && matcher != nil && matcher.Enrolled() {
		if profile := matcher.Snapshot(); profile != nil {
			mismatch = !voiceprint.Compatible(profile, voiceprint.EnrollmentContext{Device: device, EchoCancellation: echo})
		}
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) { s.VoiceprintContextMismatch = mismatch })
}

func realDevice(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && id != "default"
}
