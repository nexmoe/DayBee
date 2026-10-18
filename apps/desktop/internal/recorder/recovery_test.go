package recorder

import (
	"errors"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func waitForRecorder(t *testing.T, condition func() bool) {
	t.Helper()
	// Recovery retries share the device polling timer. Allow one full poll
	// interval; a five-second deadline races the initial poll and can never
	// observe the scheduled ten-second retry once that poll has finished.
	deadline := time.Now().Add(devicePollInterval + 5*time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("recorder did not reach the expected state")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestUnlockRetriesFailedWakeRecovery(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = "default"
	capture.mu.Unlock()
	if err := e.Resume(); err == nil || e.Status().Recording {
		t.Fatal("device startup failure was hidden", err)
	}
	if !e.Status().Recovering {
		t.Fatal("failed wake lost the pending recovery state")
	}
	capture.mu.Lock()
	capture.failID = ""
	capture.mu.Unlock()
	// Unlock can arrive after wake while the audio device is still recovering.
	if err := e.Resume(); err != nil || !e.Status().Recording {
		t.Fatal("unlock did not recover the previous recording", err)
	}
	if e.Status().Recovering {
		t.Fatal("successful recovery still appears pending")
	}
}

func TestWakeRecoveryFallsBackFromMissingSelectedMicrophone(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	settings := e.Settings()
	settings.Recording.Device = "removed-microphone"
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = settings.Recording.Device
	capture.mu.Unlock()
	if err := e.Resume(); err != nil || !e.Status().Recording {
		t.Fatal("missing selected input prevented automatic recovery", err)
	}
	if e.Settings() != settings {
		t.Fatal("automatic fallback overwrote the user's device preference")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if got := capture.options[len(capture.options)-1].DeviceID; got != "default" {
		t.Fatal("recovery did not use the available default input", got)
	}
}

func TestDisconnectedInputRetriesAfterDeviceBecomesReady(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = "default"
	fail := capture.fail
	capture.mu.Unlock()
	fail(audio.ErrDisconnected)
	waitForRecorder(t, func() bool { return e.Status().Error != nil && !e.Status().Recording })
	capture.mu.Lock()
	capture.failID = ""
	capture.mu.Unlock()
	waitForRecorder(t, func() bool { return e.Status().Recording })
	if e.Status().Error != nil {
		t.Fatal("successful recovery retained the old error")
	}
}

func TestManualStopCancelsPendingRecovery(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = "default"
	capture.mu.Unlock()
	if err := e.Resume(); err == nil {
		t.Fatal("expected a temporarily unavailable microphone")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = ""
	attempts := len(capture.options)
	capture.mu.Unlock()
	if err := e.Resume(); err != nil {
		t.Fatal(err)
	}
	e.retryRecovery(time.Now().Add(time.Minute))
	if e.Status().Recording || e.Status().Recovering || e.Busy() {
		t.Fatal("manual stop was undone by recovery")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != attempts {
		t.Fatal("canceled recovery reopened the microphone")
	}
}

func TestWakeAndUnlockLeaveIdleAndActiveRecordersAlone(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Resume(); err != nil || e.Status().Recording {
		t.Fatal("unlock started an idle recorder", err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := e.Resume(); err != nil {
			t.Fatal(err)
		}
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 1 || !e.Status().Recording {
		t.Fatal("wake/unlock reopened an already running capture")
	}
}

func TestRecoveryBackoffAndCloseCancelRetries(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = "default"
	capture.mu.Unlock()
	if err := e.Resume(); err == nil {
		t.Fatal("expected an unavailable microphone")
	}
	for range 8 {
		e.op.Lock()
		next := e.recoveryAt
		e.op.Unlock()
		capture.mu.Lock()
		attempts := len(capture.options)
		capture.mu.Unlock()
		e.retryRecovery(next.Add(-time.Nanosecond))
		capture.mu.Lock()
		tooSoon := len(capture.options) != attempts
		capture.mu.Unlock()
		if tooSoon {
			t.Fatal("retried before the scheduled delay")
		}
		e.retryRecovery(next)
		e.op.Lock()
		delay := e.recoveryAt.Sub(next)
		e.op.Unlock()
		if delay < time.Second || delay > 30*time.Second {
			t.Fatal("recovery backoff is outside its bounds", delay)
		}
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failID = ""
	attempts := len(capture.options)
	capture.mu.Unlock()
	_ = e.Resume()
	e.retryRecovery(time.Now().Add(time.Hour))
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != attempts || e.Status().Recovering {
		t.Fatal("closed recorder attempted to recover")
	}
}

func TestSystemInterruptionArchivesBothSourcesBeforeRecovery(t *testing.T) {
	e, capture, _, root := testEngine(t, true, &fakeRuntime{})
	enableSystemAudio(t, e)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		microphone := sourcePacket(protocol.SourceMicrophone, 512, .1)
		microphone.DeviceID = "default"
		if err := e.consume(microphone); err != nil {
			return err
		}
		return e.consume(sourcePacket(protocol.SourceSystem, 512, .2))
	}); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	fail := capture.fail
	capture.mu.Unlock()
	fail(audio.ErrInterrupted)
	waitForRecorder(t, func() bool {
		capture.mu.Lock()
		attempts := len(capture.options)
		capture.mu.Unlock()
		return attempts == 2 && e.Status().Recording
	})
	items := History([]string{root})
	if len(items) != 2 || items[0].AudioSource == items[1].AudioSource {
		t.Fatal("interruption lost the previous source archives", items)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.options[1].SystemAudio || capture.options[1].EchoCancellation != capture.options[0].EchoCancellation {
		t.Fatal("recovery changed the source or echo cancellation options")
	}
}

func TestTerminalCaptureErrorCannotBeResumedByUnlock(t *testing.T) {
	for _, failure := range []error{ErrPCMOverflow, errors.New("System audio capture was stopped by the user.")} {
		t.Run(failure.Error(), func(t *testing.T) {
			e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			capture.mu.Lock()
			fail := capture.fail
			capture.mu.Unlock()
			fail(failure)
			waitForRecorder(t, func() bool { return !e.Status().Recording && e.Status().Error != nil })
			_ = e.Resume()
			e.retryRecovery(time.Now().Add(time.Hour))
			if e.Status().Recording || e.Status().Recovering {
				t.Fatal("terminal stop was undone by unlock/retry")
			}
		})
	}
}

func TestRepeatedInterruptionsDoNotReopenDevicesInATightLoop(t *testing.T) {
	e, capture, _, _ := testEngine(t, true, &fakeRuntime{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Suspend(); err != nil {
		t.Fatal(err)
	}
	if err := e.Resume(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	fail := capture.fail
	capture.mu.Unlock()
	fail(audio.ErrDisconnected)
	waitForRecorder(t, func() bool { return e.Status().Recovering && !e.Status().Recording })
	e.op.Lock()
	next := e.recoveryAt
	e.op.Unlock()
	e.retryRecovery(next.Add(-time.Nanosecond))
	capture.mu.Lock()
	attempts := len(capture.options)
	capture.mu.Unlock()
	if attempts != 2 {
		t.Fatal("reopened an immediately disconnected device without a delay", attempts)
	}
	e.retryRecovery(next)
	if !e.Status().Recording {
		t.Fatal("recording did not recover after the minimum interval")
	}
}
