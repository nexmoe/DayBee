package desktop

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type enrollmentHoldVAD struct {
	held   []float32
	ready  []float32
	speech bool
}

func (v *enrollmentHoldVAD) Accept(samples []float32) {
	v.speech = len(samples) > 0 && samples[0] != 0
	if v.speech {
		v.held = append(v.held, samples...)
	}
}
func (v *enrollmentHoldVAD) Detected() bool { return v.speech }
func (v *enrollmentHoldVAD) Pop() asr.Speech {
	samples := v.ready
	v.ready = nil
	return asr.Speech{Samples: samples}
}
func (v *enrollmentHoldVAD) Flush() {
	v.ready = append(v.ready, v.held...)
	v.held = nil
}
func (v *enrollmentHoldVAD) Reset() { v.held, v.ready, v.speech = nil, nil, false }
func (v *enrollmentHoldVAD) Close() {}

func enrollmentWindow(value float32, source protocol.AudioSource) audio.Packet {
	samples := make([]float32, audio.WindowSize)
	for i := range samples {
		samples[i] = value
	}
	return audio.Packet{Samples: samples, Source: source}
}

func enrollmentRepeat(n int, value float32, source protocol.AudioSource) []audio.Packet {
	packets := make([]audio.Packet, n)
	for i := range packets {
		packets[i] = enrollmentWindow(value, source)
	}
	return packets
}

func TestEnrollmentPauseSyncsVoicedAndReferenceResumes(t *testing.T) {
	v, profiles, closed, active := enrollmentHarness(t, true, true)
	v.timeout = 4 * time.Second
	v.newVAD = func() (asr.VAD, error) { return &enrollmentHoldVAD{}, nil }
	script := append(enrollmentRepeat(13, 0, protocol.SourceMicrophone), enrollmentRepeat(40, 0.2, protocol.SourceMicrophone)...)
	script = append(script, enrollmentWindow(0.5, protocol.SourceSystem))
	script = append(script, enrollmentRepeat(8, 0.55, protocol.SourceMicrophone)...)
	gateAt := len(script) - 1
	script = append(script, enrollmentRepeat(80, 0.3, protocol.SourceMicrophone)...)
	gate, release := make(chan struct{}), make(chan struct{})
	v.newCapture = func() (enrollmentCapture, error) {
		return &enrollmentTestCapture{
			script: script, useGate: true, gateAfter: gateAt, gate: gate, release: release,
			failReferenceAfterGate: true, closed: closed, active: active,
		}, nil
	}
	done := make(chan error, 1)
	var take enrollmentTake
	go func() {
		var err error
		take, err = v.collect(context.Background(), 0, 2, false)
		done <- err
	}()
	select {
	case <-gate:
	case err := <-done:
		t.Fatal("collection finished before playback paused", err)
	case <-time.After(3 * time.Second):
		t.Fatal("playback did not pause")
	}
	deadline := time.Now().Add(time.Second)
	paused := false
	for time.Now().Before(deadline) {
		snapshot := v.Snapshot()
		if snapshot.Message == voiceprintMsgPlayback {
			paused = true
			if snapshot.SpeechSeconds != 0 {
				t.Fatalf("paused progress %v counted audio that was not kept", snapshot.SpeechSeconds)
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !paused {
		t.Fatal("playback pause was not shown")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("reference failure left enrollment paused")
	}
	if len(take.samples) < 2*audio.SampleRate || profiles.embedded.Load() != 0 || active.Load() != 0 || closed.Load() != 1 {
		t.Fatalf("samples %d embedded %d active %d closed %d", len(take.samples), profiles.embedded.Load(), active.Load(), closed.Load())
	}
	for _, sample := range take.samples {
		if sample < 0.25 || sample > 0.35 {
			t.Fatalf("paused or pre-pause audio was kept: %v", sample)
		}
	}
}

func TestReferenceLossDoesNotStayPaused(t *testing.T) {
	v, _, closed, active := enrollmentHarness(t, true, true)
	v.timeout = 3 * time.Second
	script := append(enrollmentRepeat(13, 0, protocol.SourceMicrophone), enrollmentWindow(0.5, protocol.SourceSystem))
	script = append(script, enrollmentRepeat(40, 0.25, protocol.SourceMicrophone)...)
	v.newCapture = func() (enrollmentCapture, error) {
		return &enrollmentTestCapture{script: script, failReferenceAtStart: true, closed: closed, active: active}, nil
	}
	take, err := v.collect(context.Background(), 0, 0.5, false)
	if err != nil || len(take.samples) < int(0.5*audio.SampleRate) {
		t.Fatalf("reference loss stopped recording: %d %v", len(take.samples), err)
	}
	if v.Snapshot().Message != voiceprintMsgReference || active.Load() != 0 || closed.Load() != 1 {
		t.Fatalf("message %q active %d closed %d", v.Snapshot().Message, active.Load(), closed.Load())
	}
}

func TestEnrollmentRejectsDeviceChangeOnEveryStep(t *testing.T) {
	for _, changeAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+changeAt)), func(t *testing.T) {
			v, profiles, closed, active := enrollmentHarness(t, true, true)
			var opened atomic.Int32
			v.newCapture = func() (enrollmentCapture, error) {
				device := "native:enrollment-mic"
				if int(opened.Add(1))-1 == changeAt {
					device = "native:other-mic"
				}
				return &enrollmentTestCapture{speech: true, device: device, closed: closed, active: active}, nil
			}
			if err := v.Start(); err != nil {
				t.Fatal(err)
			}
			for range changeAt {
				waitEnrollment(t, v, protocol.VoiceprintSampleReady)
				if err := v.Advance(); err != nil {
					t.Fatal(err)
				}
			}
			waitEnrollment(t, v, protocol.VoiceprintError)
			message := enrollmentError(v)
			v.Cancel()
			if message != voiceprintErrDevice || profiles.saved.Load() != 0 || profiles.embedded.Load() != int32(changeAt) || active.Load() != 0 {
				t.Fatalf("%q embedded %d saved %d", message, profiles.embedded.Load(), profiles.saved.Load())
			}
		})
	}
}

func TestEnrollmentRejectsEchoChangeBeforeCapture(t *testing.T) {
	for _, changeAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+changeAt)), func(t *testing.T) {
			v, profiles, closed, active := enrollmentHarness(t, true, true)
			if err := v.Start(); err != nil {
				t.Fatal(err)
			}
			for step := range changeAt {
				waitEnrollment(t, v, protocol.VoiceprintSampleReady)
				if step == changeAt-1 {
					settings := v.s.store.Settings()
					settings.Recording.EchoCancellation = !settings.Recording.EchoCancellation
					if _, err := v.s.store.Save(settings); err != nil {
						t.Fatal(err)
					}
				}
				if err := v.Advance(); err != nil {
					t.Fatal(err)
				}
			}
			waitEnrollment(t, v, protocol.VoiceprintError)
			message := enrollmentError(v)
			v.Cancel()
			if message != voiceprintErrDevice || profiles.saved.Load() != 0 || int(closed.Load()) != changeAt || active.Load() != 0 {
				t.Fatalf("%q captures %d", message, closed.Load())
			}
		})
	}
}

func TestEnrollmentRejectsAnInconsistentGroup(t *testing.T) {
	for _, call := range []int{1, 2, 3} {
		t.Run(string(rune('0'+call)), func(t *testing.T) {
			v, profiles, closed, _ := enrollmentHarness(t, true, true)
			profiles.outlier = call
			if err := v.Start(); err != nil {
				t.Fatal(err)
			}
			for range voiceprintSamples - 1 {
				waitEnrollment(t, v, protocol.VoiceprintSampleReady)
				if err := v.Advance(); err != nil {
					t.Fatal(err)
				}
			}
			waitEnrollment(t, v, protocol.VoiceprintError)
			message := enrollmentError(v)
			v.Cancel()
			if message != voiceprintErrInconsistent || profiles.saved.Load() != 0 || profiles.embedded.Load() != voiceprintSamples || closed.Load() != voiceprintSamples {
				t.Fatalf("%q embedded %d closed %d", message, profiles.embedded.Load(), closed.Load())
			}
		})
	}
}

func TestSettingsFailureRestoresPreviousV3Profile(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	original := enrollmentPreviousProfile(t)
	low, high, cohort := original.Calibration.TLow, original.Calibration.THigh, original.Calibration.CohortID
	if err := profiles.Manager.SaveProfile(original); err != nil {
		t.Fatal(err)
	}
	before := v.s.store.Settings()
	var saves atomic.Int32
	v.save = func(protocol.AppSettings) error {
		saves.Add(1)
		return errSettingsDisk
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	v.Cancel()
	got := profiles.Snapshot()
	if got == nil || got.Version != 3 || got.Device != "native:previous-mic" || !got.EchoCancellation || got.CreatedAt != original.CreatedAt || got.Embeddings[0][3] != 0.25 {
		t.Fatalf("restored %+v", got)
	}
	if got.Calibration == nil || got.Calibration.TLow != low || got.Calibration.THigh != high || got.Calibration.CohortID != cohort || got.Calibration.Model != voiceprint.ModelName {
		t.Fatalf("calibration %+v", got.Calibration)
	}
	if saves.Load() != 1 || v.s.store.Settings() != before || !v.Snapshot().Enrolled {
		t.Fatalf("unchanged settings were saved %d times, setup %s, enrolled %v", saves.Load(), v.s.store.Settings().Desktop.VoiceprintSetup, v.Snapshot().Enrolled)
	}
}

func TestSettingsFailureReportsRestoreError(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	if err := profiles.Manager.SaveProfile(enrollmentPreviousProfile(t)); err != nil {
		t.Fatal(err)
	}
	profiles.failRestore = true
	v.save = func(protocol.AppSettings) error { return errSettingsDisk }
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	message := enrollmentError(v)
	v.Cancel()
	if !strings.Contains(message, errSettingsDisk.Error()) || !strings.Contains(message, "voiceprint restore failed") {
		t.Fatalf("rollback error %q", message)
	}
}

func TestSettingsFailureReportsClearError(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	profiles.failClear = true
	v.save = func(protocol.AppSettings) error { return errSettingsDisk }
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	message := enrollmentError(v)
	v.Cancel()
	if !strings.Contains(message, errSettingsDisk.Error()) || !strings.Contains(message, "voiceprint clear failed") || profiles.Snapshot() == nil || v.Snapshot().Enrolled {
		t.Fatalf("clear error %q enrolled %v", message, v.Snapshot().Enrolled)
	}
}

func TestSettingsPartialWriteRestoresSettingsAndProfile(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	original := enrollmentPreviousProfile(t)
	low, high, cohort := original.Calibration.TLow, original.Calibration.THigh, original.Calibration.CohortID
	if err := profiles.Manager.SaveProfile(original); err != nil {
		t.Fatal(err)
	}
	before := v.s.store.Settings()
	var saves atomic.Int32
	v.save = func(settings protocol.AppSettings) error {
		n := saves.Add(1)
		if _, err := v.s.store.Save(settings); err != nil {
			return err
		}
		if n == 1 {
			return errors.New("login item failed")
		}
		return nil
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	message := enrollmentError(v)
	v.Cancel()
	got := profiles.Snapshot()
	if saves.Load() != 2 || !strings.Contains(message, "login item failed") || v.s.store.Settings() != before || !v.Snapshot().Enrolled {
		t.Fatalf("saves %d settings changed %v enrolled %v error %q", saves.Load(), v.s.store.Settings() != before, v.Snapshot().Enrolled, message)
	}
	if got == nil || got.Version != 3 || got.Device != "native:previous-mic" || !got.EchoCancellation || got.CreatedAt != original.CreatedAt || got.Embeddings[0][3] != 0.25 {
		t.Fatalf("restored %+v", got)
	}
	if got.Calibration == nil || got.Calibration.TLow != low || got.Calibration.THigh != high || got.Calibration.CohortID != cohort || got.Calibration.Model != voiceprint.ModelName {
		t.Fatalf("calibration %+v", got.Calibration)
	}
}

func TestSettingsRestoreFailureIsVisible(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	if err := profiles.Manager.SaveProfile(enrollmentPreviousProfile(t)); err != nil {
		t.Fatal(err)
	}
	var saves atomic.Int32
	v.save = func(settings protocol.AppSettings) error {
		n := saves.Add(1)
		if n == 1 {
			if _, err := v.s.store.Save(settings); err != nil {
				return err
			}
			return errors.New("login item failed")
		}
		return errors.New("settings restore failed")
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	message := enrollmentError(v)
	v.Cancel()
	if saves.Load() != 2 || !strings.Contains(message, "login item failed") || !strings.Contains(message, "settings restore failed") || v.Snapshot().Enrolled {
		t.Fatalf("saves %d enrolled %v error %q", saves.Load(), v.Snapshot().Enrolled, message)
	}
}

func TestRollbackFailureDoesNotResumeRecording(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	if err := profiles.Manager.SaveProfile(enrollmentPreviousProfile(t)); err != nil {
		t.Fatal(err)
	}
	profiles.failRestore = true
	v.save = func(protocol.AppSettings) error { return errSettingsDisk }
	if err := v.s.engine.Start(); err != nil {
		t.Fatal(err)
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	deadline := time.Now().Add(2 * time.Second)
	for v.Active() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if v.Active() || v.s.engine.Status().Recording || enrollmentError(v) == "" || v.Snapshot().Enrolled {
		t.Fatalf("recording %v active %v enrolled %v error %q", v.s.engine.Status().Recording, v.Active(), v.Snapshot().Enrolled, enrollmentError(v))
	}
	v.Cancel()
}

func enrollmentPreviousProfile(t *testing.T) *voiceprint.Profile {
	t.Helper()
	original, err := voiceprint.Calibrate(
		[][][]float32{enrollmentDirection(0), enrollmentDirection(0.01), enrollmentDirection(0.02)},
		voiceprint.EnrollmentContext{Device: "native:previous-mic", EchoCancellation: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	original.CreatedAt = "2000-01-01T00:00:00Z"
	original.Embeddings[0][3] = 0.25
	return original
}

func enrollmentDirection(offset float32) [][]float32 {
	vectors := make([][]float32, 4)
	for i := range vectors {
		vectors[i] = make([]float32, 192)
		vectors[i][0] = 1
		vectors[i][1] = offset + float32(i)*0.0001
	}
	return vectors
}

func enrollmentError(v *voiceprintEnrollment) string {
	if text := v.Snapshot().Error; text != nil {
		return *text
	}
	return ""
}

var errSettingsDisk = errString("settings disk unavailable")

type errString string

func (e errString) Error() string { return string(e) }
