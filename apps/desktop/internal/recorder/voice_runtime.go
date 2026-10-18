package recorder

import (
	"strings"

	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func (e *Engine) noteVoiceContext(device string) {
	if e == nil || !realDevice(device) {
		return
	}
	e.applyVoiceRuntime(strings.TrimSpace(device), e.Settings().Recording.EchoCancellation, false)
}

// rearmVoiceRuntime starts a generation for Start or a replaced manager.
func (e *Engine) rearmVoiceRuntime() {
	if e == nil {
		return
	}
	e.applyVoiceRuntime(e.resolvedVoiceDevice(), e.Settings().Recording.EchoCancellation, true)
}

// forceVoiceRuntime invalidates permits already queued for the previous context.
func (e *Engine) forceVoiceRuntime() {
	if e == nil {
		return
	}
	e.applyVoiceRuntime(e.resolvedVoiceDevice(), e.Settings().Recording.EchoCancellation, true)
}

func (e *Engine) applyVoiceRuntime(device string, echo bool, force bool) {
	next := voiceprint.EnrollmentContext{Device: strings.TrimSpace(device), EchoCancellation: echo}
	if !force && e.voiceReady && e.voiceCtx == next {
		return
	}
	if realDevice(next.Device) {
		e.voiceReady = true
		e.voiceCtx = next
	} else if force {
		e.voiceReady = false
	}
	if !force && !realDevice(next.Device) {
		return
	}
	e.resetVoiceRuntime(next)
}

func (e *Engine) resolvedVoiceDevice() string {
	if e.voiceReady && realDevice(e.voiceCtx.Device) {
		return e.voiceCtx.Device
	}
	if realDevice(e.activeDeviceID()) {
		return strings.TrimSpace(e.activeDeviceID())
	}
	return ""
}

func (e *Engine) resetVoiceRuntime(ctx voiceprint.EnrollmentContext) {
	e.mu.RLock()
	matcher := e.voice
	e.mu.RUnlock()
	resetter, ok := matcher.(voiceRuntime)
	if !ok || resetter == nil {
		return
	}
	resetter.ResetRuntime(ctx)
}
