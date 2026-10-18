package recorder

import (
	"errors"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type denyRefCapture struct{ fakeCapture }

func (c *denyRefCapture) Start(options audio.CaptureOptions, callback func(audio.Packet), fail func(error)) error {
	if err := c.fakeCapture.Start(options, callback, fail); err != nil {
		return err
	}
	if options.SystemReference && !options.SystemAudio && options.OnReferenceUnavailable != nil {
		options.OnReferenceUnavailable(errors.New("system reference permission denied"))
	}
	return nil
}

func lastOptions(t *testing.T, capture *fakeCapture) audio.CaptureOptions {
	t.Helper()
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) == 0 {
		t.Fatal("capture was not started")
	}
	return capture.options[len(capture.options)-1]
}

func TestReferenceOnlySkipsSystemArchiveAndDecode(t *testing.T) {
	runtime := &fakeRuntime{}
	e, capture, _, root := testEngine(t, false, runtime)
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceStarting {
		t.Fatal(e.Status().VoiceprintReference)
	}
	options := lastOptions(t, capture)
	if !options.SystemReference || options.SystemAudio {
		t.Fatalf("reference request = %+v", options)
	}
	if err := e.call(func() error {
		return e.consume(sourcePacket(protocol.SourceSystem, audio.WindowSize, 0.2))
	}); err != nil {
		t.Fatal(err)
	}
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceAvailable {
		t.Fatal(e.Status().VoiceprintReference)
	}
	stopSoon(t, e)
	if files, err := AudioFiles([]string{root}); err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.recognizers != 0 {
		t.Fatal(runtime.recognizers)
	}
}

func TestReferenceDeniedKeepsMicrophone(t *testing.T) {
	dir := t.TempDir()
	settings := protocol.DefaultSettings(dir)
	settings.Recording.AudioFormat = protocol.WAV
	settings.Recording.AutoSwitchDevice = false
	capture := &denyRefCapture{}
	runtime := &fakeRuntime{}
	e := New(settings, capture, &fakeAssets{}, runtime, nil, nil)
	t.Cleanup(func() { _ = e.Close() })
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording || e.Status().VoiceprintReference != protocol.VoiceprintReferenceUnavailable {
		t.Fatalf("reference denial stopped mic: %+v", e.Status())
	}
	again := lastOptions(t, &capture.fakeCapture).OnReferenceUnavailable
	done := make(chan struct{})
	go func() {
		defer close(done)
		again(errors.New("again"))
	}()
	if err := e.call(func() error {
		return e.consume(sourcePacket(protocol.SourceMicrophone, audio.WindowSize, 0.1))
	}); err != nil {
		t.Fatal(err)
	}
	<-done
	stopSoon(t, e)
	items := History([]string{dir})
	if len(items) != 1 || items[0].AudioSource != protocol.SourceMicrophone {
		t.Fatalf("mic did not continue: %+v", items)
	}
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceOff {
		t.Fatal(e.Status().VoiceprintReference)
	}
}

func TestSavedSystemAudioStillFailsStrictly(t *testing.T) {
	e, capture, _, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	capture.failSystem = true
	capture.mu.Unlock()
	previous := e.Settings()
	next := previous
	next.Recording.RecordSystemAudio = true
	if err := e.Apply(next); err == nil || e.Status().Recording || e.Settings() != previous {
		t.Fatal("system archive failure was degraded", err, e.Status().Recording)
	}
}

func TestClearVoiceprintAndDisableASRStopReference(t *testing.T) {
	e, capture, _, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if !lastOptions(t, capture).SystemReference {
		t.Fatal("reference was not requested")
	}
	e.SetVoiceprint(nil)
	if !e.Status().Recording || lastOptions(t, capture).SystemReference || e.Status().VoiceprintReference != protocol.VoiceprintReferenceOff {
		t.Fatalf("cleared voiceprint kept reference: recording=%v ref=%v phase=%s", e.Status().Recording, lastOptions(t, capture).SystemReference, e.Status().VoiceprintReference)
	}
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	settings := e.Settings()
	settings.Recording.DisableASR = true
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording || lastOptions(t, capture).SystemReference {
		t.Fatalf("disabled ASR stopped mic or kept reference: %+v", lastOptions(t, capture))
	}
}

func TestDeviceChangeClearsRingAndAnchor(t *testing.T) {
	e, _, _, _ := testEngine(t, true, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	silent := &voiceprint.Reference{Samples: []float32{0}, Complete: true}
	if err := e.call(func() error {
		e.echoRef.AppendAt(start, []float32{0.4})
		voiceAnchorFor(e).observe("session", "mic-1", start, silent, false, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	settings := e.Settings()
	settings.Recording.EchoCancellation = !settings.Recording.EchoCancellation
	if err := e.Apply(settings); err != nil {
		t.Fatal(err)
	}
	if e.echoRef.Slice(start, start.Add(time.Second)) != nil {
		t.Fatal("device change kept the reference ring")
	}
	if voiceAnchorFor(e).allow("session", "mic-1", start, silent) {
		t.Fatal("device change kept the relaxation anchor")
	}
}

func TestRealDeviceUpdatesContextMismatch(t *testing.T) {
	e, _, _, _ := testEngine(t, true, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: calibratedProfile("mic-1", false)})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	packet := sourcePacket(protocol.SourceMicrophone, 32, 0.1)
	packet.DeviceID = "mic-2"
	if err := e.call(func() error { return e.consume(packet) }); err != nil {
		t.Fatal(err)
	}
	if !e.Status().VoiceprintContextMismatch {
		t.Fatal("calibrated profile matched a different microphone")
	}
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	packet.DeviceID = "mic-3"
	if err := e.call(func() error { return e.consume(packet) }); err != nil {
		t.Fatal(err)
	}
	if e.Status().VoiceprintContextMismatch {
		t.Fatal("legacy profile was treated as device-bound")
	}
}

func TestReferenceGapIsNotSilent(t *testing.T) {
	ring := newEchoReference()
	start := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	ring.AppendAt(start, make([]float32, 100))
	ring.AppendAt(echoAt(start, 150), make([]float32, 50))
	gapped, complete := ring.SliceWithCoverage(start, echoAt(start, 200))
	if complete || referenceSilent(&voiceprint.Reference{Samples: gapped, Complete: complete}) {
		t.Fatal("a zero-filled gap counted as silence")
	}
	full, ok := ring.SliceWithCoverage(start, echoAt(start, 100))
	if !ok || !referenceSilent(&voiceprint.Reference{Samples: full, Complete: true}) {
		t.Fatal("fully observed zeros were not silence")
	}
	if referenceSilent(nil) || referenceSilent(&voiceprint.Reference{Samples: full}) {
		t.Fatal("nil or incomplete reference was treated as silence")
	}
}

func TestAlignPiecesCompleteOnlyWhenCovered(t *testing.T) {
	start := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	end := echoAt(start, 4)
	partial := []speechPiece{{samples: []float32{1, 1}, started: start.Format(time.RFC3339Nano), ended: echoAt(start, 2).Format(time.RFC3339Nano)}}
	samples, complete := alignPieces(partial, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), 4)
	if samples == nil || complete {
		t.Fatal(samples, complete)
	}
	full := []speechPiece{{samples: []float32{1, 1, 1, 1}, started: start.Format(time.RFC3339Nano), ended: end.Format(time.RFC3339Nano)}}
	if _, complete = alignPieces(full, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), 4); !complete {
		t.Fatal("full system coverage was incomplete")
	}
}

func TestFrozenReferenceIgnoresLaterRingWrites(t *testing.T) {
	e, _, _, _ := testEngine(t, false, &fakeRuntime{})
	voice := &scriptVoice{profile: acceptedLegacy(), hold: make(chan struct{}), release: make(chan struct{})}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	voice.mu.Lock()
	started := voice.hold
	voice.mu.Unlock()
	var speechStart time.Time
	if err := e.call(func() error {
		speechStart = e.segment.start
		e.echoRef.AppendAt(speechStart, make([]float32, 16000))
		return e.appendSpeech(&e.audioStream, asr.Speech{Samples: make([]float32, 16000)})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("match did not start")
	}
	loud := make([]float32, 16000)
	for i := range loud {
		loud[i] = 0.8
	}
	e.echoRef.AppendAt(speechStart, loud)
	close(voice.release)
	stopSoon(t, e)
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if !voice.frozenComplete || len(voice.frozenRef) != 16000 || voice.frozenRef[0] != 0 {
		t.Fatalf("reference was not frozen: complete=%v len=%d", voice.frozenComplete, len(voice.frozenRef))
	}
}

func TestLateReferenceCallbackCannotReviveState(t *testing.T) {
	e, capture, _, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	callback := lastOptions(t, capture).OnReferenceUnavailable
	stopSoon(t, e)
	callback(errors.New("late"))
	if e.Status().VoiceprintReference != protocol.VoiceprintReferenceOff {
		t.Fatal(e.Status().VoiceprintReference)
	}
}

func TestClearSameManagerStopsReferenceOnApply(t *testing.T) {
	e, capture, _, _ := testEngine(t, false, &fakeRuntime{})
	manager, err := voiceprint.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if err = manager.Save(acceptedLegacy().Embeddings); err != nil {
		t.Fatal(err)
	}
	e.SetVoiceprint(manager)
	if err = e.Start(); err != nil {
		t.Fatal(err)
	}
	if !lastOptions(t, capture).SystemReference {
		t.Fatal("reference was not requested")
	}
	if err = e.call(func() error {
		e.echoRef.AppendAt(time.Now(), []float32{0.4})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = manager.Clear(); err != nil || manager.Enrolled() {
		t.Fatal("clear", err, manager.Enrolled())
	}
	if err = e.Apply(e.Settings()); err != nil {
		t.Fatal(err)
	}
	if !e.Status().Recording || lastOptions(t, capture).SystemReference || e.Status().VoiceprintReference != protocol.VoiceprintReferenceOff {
		t.Fatalf("cleared manager kept reference: recording=%v ref=%v phase=%s", e.Status().Recording, lastOptions(t, capture).SystemReference, e.Status().VoiceprintReference)
	}
	if e.echoRef.Slice(time.Now().Add(-time.Minute), time.Now().Add(time.Minute)) != nil {
		t.Fatal("cleared manager left system audio in the ring")
	}
}

func TestSavingSettingsDoesNotRetryUnavailableReference(t *testing.T) {
	dir := t.TempDir()
	settings := protocol.DefaultSettings(dir)
	settings.Recording.AudioFormat = protocol.WAV
	settings.Recording.AutoSwitchDevice = false
	capture := &denyRefCapture{}
	e := New(settings, capture, &fakeAssets{}, &fakeRuntime{}, nil, nil)
	t.Cleanup(func() { _ = e.Close() })
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	starts := len(capture.options)
	capture.mu.Unlock()
	if err := e.Apply(e.Settings()); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != starts || !e.Status().Recording || e.Status().VoiceprintReference != protocol.VoiceprintReferenceUnavailable {
		t.Fatalf("unavailable reference was restarted: starts %d->%d recording=%v phase=%s", starts, len(capture.options), e.Status().Recording, e.Status().VoiceprintReference)
	}
}
