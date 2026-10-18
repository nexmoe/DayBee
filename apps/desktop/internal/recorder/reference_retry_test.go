package recorder

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

type recoverableReferenceCapture struct {
	fakeCapture
	denied atomic.Bool
}

func (c *recoverableReferenceCapture) Start(options audio.CaptureOptions, consume func(audio.Packet), fail func(error)) error {
	if err := c.fakeCapture.Start(options, consume, fail); err != nil {
		return err
	}
	if c.denied.Load() && options.OnReferenceUnavailable != nil {
		options.OnReferenceUnavailable(errors.New("screen recording permission denied"))
	}
	return nil
}

func TestRetryReferenceRecoversWithoutEndingTheRecording(t *testing.T) {
	root := t.TempDir()
	settings := protocol.DefaultSettings(root)
	settings.Recording.AudioFormat = protocol.WAV
	settings.Recording.Device = "default"
	settings.Recording.AutoSwitchDevice = false
	capture := &recoverableReferenceCapture{}
	capture.denied.Store(true)
	e := New(settings, capture, &fakeAssets{}, &fakeRuntime{}, nil, nil)
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceUnavailable {
		t.Fatal("did not reproduce unavailable reference")
	}
	if e.Status().VoiceprintReferenceError != "screen recording permission denied" {
		t.Fatal("reference failure reason was discarded")
	}
	retry, ok := any(e).(interface{ RetryVoiceprintReference() error })
	if !ok {
		t.Fatal("failed reference has no recovery entry point while recording")
	}
	first := lastOptions(t, &capture.fakeCapture)
	var segmentStart time.Time
	var segmentID uint64
	if err := e.call(func() error {
		if err := e.consume(sourcePacket(protocol.SourceMicrophone, audio.WindowSize, .1)); err != nil {
			return err
		}
		segmentStart, segmentID = e.segment.start, e.segment.id
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	capture.denied.Store(false)
	if err := retry.RetryVoiceprintReference(); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording || e.Status().VoiceprintReference != protocol.VoiceprintReferenceStarting {
		t.Fatalf("retry did not restart the check: %+v", e.Status())
	}
	if e.Status().VoiceprintReferenceError != "" {
		t.Fatal("retry retained a stale error")
	}
	first.OnReferenceUnavailable(errors.New("late error from old reference"))
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceStarting {
		t.Fatal("old failure overwrote recovery")
	}
	if err := e.call(func() error {
		if e.segment.id != segmentID || !e.segment.start.Equal(segmentStart) {
			return errors.New("reference recovery changed the recording or its timestamps")
		}
		if err := e.consume(sourcePacket(protocol.SourceSystem, audio.WindowSize, 0)); err != nil {
			return err
		}
		return e.consume(sourcePacket(protocol.SourceMicrophone, audio.WindowSize, .1))
	}); err != nil {
		t.Fatal(err)
	}
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceAvailable {
		t.Fatal("recovered audio was not accepted")
	}
	stopSoon(t, e)
	if items := History([]string{root}); len(items) != 1 || len(items[0].CapturedRanges) != 2 {
		t.Fatalf("recording was split or lost speech: %+v", items)
	}
}

func TestRetryReferenceDoesNotStartIdleRecording(t *testing.T) {
	e, capture, _, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.RetryVoiceprintReference(); err == nil || e.Status().Recording {
		t.Fatal("retry started a recording the user had not started")
	}
	if len(capture.options) != 0 {
		t.Fatal("idle retry requested audio capture")
	}
}
