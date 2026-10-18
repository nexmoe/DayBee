package recorder

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type seenSpan struct {
	device   string
	echo     bool
	complete bool
	hasRef   bool
	revision uint64
	profile  bool
}

type contextVoice struct {
	scriptVoice
	mu       sync.Mutex
	resets   []voiceprint.EnrollmentContext
	seen     []seenSpan
	revision uint64
	pool     int
	relaxed  int
	gate     chan struct{}
	resume   chan struct{}
	finished chan struct{}
}

func (c *contextVoice) Snapshot() *voiceprint.Profile {
	profile := c.scriptVoice.Snapshot()
	if profile == nil {
		return nil
	}
	c.mu.Lock()
	profile.Revision = c.revision
	c.mu.Unlock()
	return profile
}

func (c *contextVoice) ResetRuntime(ctx voiceprint.EnrollmentContext) {
	c.mu.Lock()
	c.revision++
	c.pool = 0
	c.resets = append(c.resets, ctx)
	c.mu.Unlock()
}

func (c *contextVoice) MatchSpansRelaxed(profile *voiceprint.Profile, samples []float32, rate int, ref *voiceprint.Reference, relax bool) []voiceprint.IdentitySpan {
	c.mu.Lock()
	c.relaxed++
	c.mu.Unlock()
	return c.scriptVoice.MatchSpansRelaxed(profile, samples, rate, ref, relax)
}

func (c *contextVoice) MatchSpansContext(profile *voiceprint.Profile, samples []float32, rate int, ref *voiceprint.Reference, ctx voiceprint.EnrollmentContext, relax bool) []voiceprint.IdentitySpan {
	c.mu.Lock()
	item := seenSpan{device: ctx.Device, echo: ctx.EchoCancellation, hasRef: ref != nil, profile: profile != nil}
	if ref != nil {
		item.complete = ref.Complete
	}
	if profile != nil {
		item.revision = profile.Revision
	}
	c.seen = append(c.seen, item)
	gate, resume, finished := c.gate, c.resume, c.finished
	c.gate, c.resume, c.finished = nil, nil, nil
	c.mu.Unlock()
	if gate != nil {
		close(gate)
		<-resume
	}
	c.mu.Lock()
	if profile != nil && profile.Revision == c.revision && ref != nil && ref.Complete {
		c.pool++
	}
	c.mu.Unlock()
	spans := c.scriptVoice.MatchSpansRelaxed(profile, samples, rate, ref, relax)
	if finished != nil {
		close(finished)
	}
	return spans
}

func (c *contextVoice) counts() (resets int, pool int, seen []seenSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.resets), c.pool, append([]seenSpan(nil), c.seen...)
}

func quietEcho(t *testing.T, e *Engine) {
	t.Helper()
	settings := e.Settings()
	if !settings.Recording.EchoCancellation {
		return
	}
	settings.Recording.EchoCancellation = false
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
}

func TestLiveMatchSpansContextReceivesFrozenCapture(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	quietEcho(t, e)
	voice := &contextVoice{scriptVoice: scriptVoice{profile: acceptedLegacy()}}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	voice.mu.Lock()
	voice.finished = make(chan struct{})
	finished := voice.finished
	voice.mu.Unlock()
	if err := e.call(func() error {
		e.echoRef.AppendAt(e.segment.start, make([]float32, audio.WindowSize))
		speech := sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.2)
		speech.DeviceID = "mic-a"
		return e.consume(speech)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("context match did not finish")
	}
	resets, pool, seen := voice.counts()
	if resets != 2 || pool != 1 || len(seen) != 1 {
		t.Fatalf("resets=%d pool=%d calls=%d", resets, pool, len(seen))
	}
	if !seen[0].profile || !seen[0].hasRef || !seen[0].complete || seen[0].device != "mic-a" || seen[0].echo {
		t.Fatalf("frozen capture = %+v", seen[0])
	}
	if err := e.call(func() error {
		again := sourcePacket(protocol.SourceMicrophone, 32, 0.1)
		again.DeviceID = "mic-a"
		if err := e.consume(again); err != nil {
			return err
		}
		blank := sourcePacket(protocol.SourceMicrophone, 32, 0.1)
		blank.DeviceID = "default"
		if err := e.consume(blank); err != nil {
			return err
		}
		moved := sourcePacket(protocol.SourceMicrophone, 32, 0.1)
		moved.DeviceID = "mic-b"
		return e.consume(moved)
	}); err != nil {
		t.Fatal(err)
	}
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if len(voice.resets) != 3 || voice.resets[1].Device != "mic-a" || voice.resets[2].Device != "mic-b" || voice.resets[2].EchoCancellation {
		t.Fatalf("resets = %+v", voice.resets)
	}
}

func TestDeviceChangeResetsRuntimeBeforeStaleLearning(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	quietEcho(t, e)
	voice := &contextVoice{scriptVoice: scriptVoice{profile: acceptedLegacy()}}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	voice.mu.Lock()
	voice.finished = make(chan struct{})
	first := voice.finished
	voice.mu.Unlock()
	if err := e.call(func() error {
		e.echoRef.AppendAt(e.segment.start, make([]float32, audio.WindowSize*2))
		packet := sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.2)
		packet.DeviceID = "mic-a"
		return e.consume(packet)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("first span did not finish")
	}
	voice.mu.Lock()
	voice.gate = make(chan struct{})
	voice.resume = make(chan struct{})
	voice.finished = make(chan struct{})
	gate, resume, second := voice.gate, voice.resume, voice.finished
	voice.mu.Unlock()
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
	})
	if err := e.call(func() error {
		packet := sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.2)
		packet.DeviceID = "mic-a"
		return e.consume(packet)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate:
	case <-time.After(2 * time.Second):
		t.Fatal("second span did not reach the runtime")
	}
	settings := e.Settings()
	settings.Recording.EchoCancellation = true
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	_, pool, seen := voice.counts()
	if pool != 0 || len(seen) != 2 || seen[1].echo || seen[1].device != "mic-a" {
		t.Fatalf("stale call pool=%d seen=%+v", pool, seen)
	}
	close(resume)
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("stale span did not finish")
	}
	if _, pool, _ = voice.counts(); pool != 0 {
		t.Fatalf("stale span wrote the new pool: %d", pool)
	}
	voice.mu.Lock()
	voice.finished = make(chan struct{})
	third := voice.finished
	voice.mu.Unlock()
	if err := e.call(func() error {
		e.echoRef.AppendAt(e.segment.start, make([]float32, audio.WindowSize))
		packet := sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.2)
		packet.DeviceID = "mic-a"
		return e.consume(packet)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-third:
	case <-time.After(2 * time.Second):
		t.Fatal("new span did not finish")
	}
	_, pool, seen = voice.counts()
	if pool != 1 || len(seen) != 3 || !seen[2].echo || !seen[2].complete || seen[2].revision == seen[1].revision {
		t.Fatalf("new context pool=%d seen=%+v", pool, seen)
	}
}

func TestLegacyMatcherKeepsRelaxedSpans(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	voice := &scriptVoice{profile: acceptedLegacy()}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		return e.consume(sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.2))
	}); err != nil {
		t.Fatal(err)
	}
	stopSoon(t, e)
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if len(voice.relaxed) == 0 {
		t.Fatal("legacy matcher was not used")
	}
}

func TestBatchSpanMatchDoesNotLearn(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	voice := &contextVoice{scriptVoice: scriptVoice{profile: acceptedLegacy()}}
	installVoice(e, voice)
	now := time.Now()
	piece := speechPiece{samples: make([]float32, 160), started: now.Format(time.RFC3339Nano), ended: now.Add(time.Second).Format(time.RFC3339Nano)}
	_, _, _, err := e.labelOnePiece(context.Background(), voice, voice.Snapshot(), piece, &voiceprint.Reference{Samples: []float32{0}, Complete: true}, voiceprint.EnrollmentContext{Device: "mic-a", EchoCancellation: true}, true, "session", now, &voiceAnchor{}, e.assets.ASRPath())
	if err != nil {
		t.Fatal(err)
	}
	resets, pool, seen := voice.counts()
	voice.mu.Lock()
	relaxed := voice.relaxed
	voice.mu.Unlock()
	if resets != 0 || pool != 0 || len(seen) != 0 || relaxed != 1 {
		t.Fatalf("batch learned: resets=%d pool=%d context=%d relaxed=%d", resets, pool, len(seen), relaxed)
	}
}
