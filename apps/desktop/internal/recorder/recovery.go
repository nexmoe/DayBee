package recorder

import (
	"log/slog"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// interrupted runs on the audio loop, which must remain free to consume commands
// while finalizers finish. Capture is closed before any attempt to reopen it.
func (e *Engine) interrupted(err error) {
	if e.vad == nil {
		return
	}
	if stopErr := e.stop(); stopErr != nil {
		e.safeStop(stopErr)
		return
	}
	slog.Warn("Audio capture interrupted", "error", err)
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.Recording, s.InSpeech, s.Recovering = false, false, true
		s.StatusMessage = "Recording interrupted. Reconnecting…"
	})
	go func() {
		e.op.Lock()
		defer e.op.Unlock()
		now := time.Now()
		if !now.Before(e.recoveryAt) {
			_ = e.recoverLocked(now)
		}
	}()
}

// retryRecovery shares the existing device polling timer. Failed attempts back
// off to at most one every 30 seconds, with no extra timers or retry goroutines.
func (e *Engine) retryRecovery(now time.Time) {
	e.op.Lock()
	defer e.op.Unlock()
	if e.Status().Recovering && !now.Before(e.recoveryAt) {
		_ = e.recoverLocked(now)
	}
}

// The caller holds op. Only recordings the user already started can recover;
// Stop, Close and terminal recording errors clear intent and prevent reopening.
func (e *Engine) recoverLocked(now time.Time) error {
	if e.closed || !e.intent || e.suspended || e.halts.Load() > 0 || e.Status().Recording {
		return nil
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) { s.Recovering = true })
	e.finalizers.Wait()
	settings := e.Settings()
	err := e.prepare(true, !settings.Recording.DisableASR)
	if err == nil {
		// A Bluetooth/USB device can disappear across sleep. Preserve the saved
		// preference, but allow this recovery to use the available default input.
		if devices, deviceErr := e.capture.Devices(); deviceErr == nil && settings.Recording.Device != "default" {
			found := false
			for _, device := range devices {
				found = found || device.ID == settings.Recording.Device
			}
			if !found {
				settings.Recording.Device = "default"
			}
		}
		err = e.call(func() error { return e.start(settings) })
	}
	if err != nil {
		e.recoveryDelay = min(30*time.Second, max(time.Second, e.recoveryDelay*2))
		e.recoveryAt = now.Add(e.recoveryDelay)
		e.fail(err)
		e.patch(func(s *protocol.RecorderStatusSnapshot) {
			s.Recording, s.InSpeech, s.Recovering = false, false, true
			s.StatusMessage = "Recording interrupted. Retrying automatically…"
		})
		slog.Warn("Recording recovery scheduled", "delay", e.recoveryDelay)
		return err
	}
	// A device may open successfully and immediately disconnect again. Keep a
	// minimum interval between automatic opens, including those "successes".
	e.recoveryAt, e.recoveryDelay = now.Add(3*time.Second), 0
	slog.Info("Recording resumed automatically", "device", settings.Recording.Device)
	return nil
}
