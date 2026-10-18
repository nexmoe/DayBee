package recorder

import (
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func (e *Engine) considerAutoSwitch(policy *audio.SwitchPolicy, settings protocol.RecordingSettings,
	current string, devices []protocol.DeviceInfo, currentRMS float64, now time.Time) {
	reset := func() { policy.Consider(now, "", 0, 0, settings.AutoSwitchConfirmations) }
	if !settings.AutoSwitchDevice {
		reset()
		return
	}
	candidates := policy.One(audio.Candidates(devices, current, settings.ExcludeDeviceKeywords))
	if len(candidates) == 0 {
		reset()
		return
	}
	if settings.EchoCancellation {
		// The live level has passed through AEC, while Probe uses raw capture.
		// Compare raw with raw so cancellation/gain cannot bias the selection.
		// Probes open a temporary input; they never stop the live AEC engine.
		var err error
		currentRMS, err = e.capture.Probe(e.ctx, current)
		if err != nil {
			reset()
			return
		}
	}
	bestID, bestRMS := "", 0.0
	for _, device := range candidates {
		rms, err := e.capture.Probe(e.ctx, device.ID)
		if err == nil && rms > bestRMS {
			bestID, bestRMS = device.ID, rms
		}
	}
	// A user may disable switching or change the selected input while a probe
	// is in progress. Discard that stale decision instead of overriding them.
	latest := e.Settings().Recording
	if !latest.AutoSwitchDevice || latest.Device != settings.Device || latest.EchoCancellation != settings.EchoCancellation {
		reset()
		return
	}
	if policy.Consider(now, bestID, bestRMS, currentRMS, settings.AutoSwitchConfirmations) {
		e.switchDevice(bestID)
	}
}
