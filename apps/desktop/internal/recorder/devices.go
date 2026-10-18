package recorder

import (
	"log/slog"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
)

const (
	devicePollInterval = 10 * time.Second
	autoSwitchQuiet    = 10 * time.Second
	autoSwitchQuietRMS = 0.01
)

func (e *Engine) pollDevices() {
	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()
	policy := audio.SwitchPolicy{}
	var quietSince time.Time
	for {
		devices, err := e.capture.Devices()
		if err == nil {
			e.mu.Lock()
			e.devices = devices
			e.mu.Unlock()
			e.notify()
			settings, status := e.Settings(), e.Status()
			if status.Recording {
				e.mu.RLock()
				current := e.activeDevice
				e.mu.RUnlock()
				if current == "" {
					current = settings.Recording.Device
				}
				found := current == "default"
				for _, d := range devices {
					if d.ID == current {
						found = true
					}
					if current == "default" && d.IsDefault {
						current = d.ID
					}
				}
				if !found {
					e.switchDevice("default")
					slog.Warn("Selected microphone disconnected; using default")
				} else if settings.Recording.AutoSwitchDevice {
					now := time.Now()
					if status.RMS >= autoSwitchQuietRMS {
						quietSince = time.Time{}
						policy.Consider(now, "", 0, 0, settings.Recording.AutoSwitchConfirmations)
					} else if quietSince.IsZero() {
						quietSince = now
					} else if now.Sub(quietSince) >= autoSwitchQuiet {
						e.considerAutoSwitch(&policy, settings.Recording, current, devices, status.RMS, now)
					}
				}
			}
		}
		e.retryRecovery(time.Now())
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (e *Engine) switchDevice(id string) {
	e.op.Lock()
	defer e.op.Unlock()
	if e.closed || !e.Status().Recording {
		return
	}
	err := e.call(func() error {
		if e.vad == nil {
			return nil
		}
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
				goto drained
			}
		}
	drained:
		if err := e.rotate(); err != nil {
			return err
		}
		e.newSegment()
		return e.startCapture(id, true)
	})
	if err != nil {
		e.fail(err)
		_ = e.stopAndWait()
		_ = e.recoverLocked(time.Now())
	} else {
		slog.Info("Microphone switched", "device", id)
	}
}
