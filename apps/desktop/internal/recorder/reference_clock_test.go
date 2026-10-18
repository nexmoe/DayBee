package recorder

import (
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func TestConsumeArrivalGapBreaksCoverage(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(map[bool]string{false: "reference-only", true: "saved-system"}[saved], func(t *testing.T) {
			e, _, _, _ := testEngine(t, false, &fakeRuntime{})
			installVoice(e, &scriptVoice{profile: acceptedLegacy()})
			if saved {
				enableSystemAudio(t, e)
			}
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			base := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
			const n = 200
			first := sourcePacket(protocol.SourceSystem, n, 0)
			first.CapturedAt = base
			second := sourcePacket(protocol.SourceSystem, n, 0)
			second.CapturedAt = echoAt(base, n).Add(200 * time.Millisecond)
			third := sourcePacket(protocol.SourceSystem, n, 0)
			third.CapturedAt = echoAt(second.CapturedAt, n).Add(20 * time.Millisecond)
			var gapped, joined bool
			if err := e.call(func() error {
				for _, packet := range []audio.Packet{first, second, third} {
					if err := e.consume(packet); err != nil {
						return err
					}
				}
				_, gapped = e.echoRef.SliceWithCoverage(base, echoAt(second.CapturedAt, n))
				_, joined = e.echoRef.SliceWithCoverage(second.CapturedAt, echoAt(second.CapturedAt, n*2))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if gapped || !joined {
				t.Fatalf("gap complete=%v jitter complete=%v", gapped, joined)
			}
			hole, _ := e.echoRef.SliceWithCoverage(base, echoAt(second.CapturedAt, n))
			if referenceSilent(&voiceprint.Reference{Samples: hole, Complete: gapped}) {
				t.Fatal("arrival gap counted as silence")
			}
		})
	}
}

func TestStopDrainsStampedQueueBeforeRetiringReference(t *testing.T) {
	e, _, _, root := testEngine(t, false, &fakeRuntime{})
	voice := &scriptVoice{profile: acceptedLegacy()}
	installVoice(e, voice)
	enableSystemAudio(t, e)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		epoch := e.refEpoch.Load()
		mic := sourcePacket(protocol.SourceMicrophone, 100, 0.4)
		mic.CaptureEpoch = epoch
		sys := sourcePacket(protocol.SourceSystem, audio.WindowSize, 0)
		sys.CaptureEpoch = epoch
		sys.CapturedAt = e.segment.start
		e.packets <- mic
		e.packets <- sys
		return e.stop()
	}); err != nil {
		t.Fatal(err)
	}
	e.finalizers.Wait()
	voice.mu.Lock()
	complete, ref := voice.frozenComplete, append([]float32(nil), voice.frozenRef...)
	voice.mu.Unlock()
	if !complete || len(ref) != audio.WindowSize || ref[0] != 0 {
		t.Fatalf("tail reference lost: complete=%v len=%d", complete, len(ref))
	}
	items := History([]string{root})
	if len(items) != 2 {
		t.Fatalf("tail archives = %+v", items)
	}
	for _, item := range items {
		samples := archivedSamples(t, item.AudioPath)
		if len(samples) != audio.WindowSize {
			t.Fatalf("%s pcm samples %d", item.AudioSource, len(samples))
		}
		sample := samples[0]
		level := float32(0.4)
		want := float32(0)
		if item.AudioSource == protocol.SourceMicrophone {
			want = float32(int16(float64(level)*32767)) / 32768
		}
		if sample != want {
			t.Fatalf("%s first sample %v", item.AudioSource, sample)
		}
	}
}

func TestReferenceRestartKeepsQueuedMicrophone(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		packet := sourcePacket(protocol.SourceMicrophone, 80, 0.3)
		packet.CaptureEpoch = e.refEpoch.Load()
		e.packets <- packet
		if err := e.restartCaptureOnly(e.Settings().Recording.Device); err != nil {
			return err
		}
		if len(e.remainder) != 80 || e.remainder[0] != 0.3 {
			t.Fatalf("queued microphone dropped: %d", len(e.remainder))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
