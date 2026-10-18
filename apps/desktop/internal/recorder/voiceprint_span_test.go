package recorder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type countRuntime struct {
	fakeRuntime
	mu     sync.Mutex
	sizes  []int
	failAt int
}

func (r *countRuntime) NewRecognizer(string) (asr.Recognizer, error) {
	return &countRecognizer{runtime: r}, nil
}

type countRecognizer struct{ runtime *countRuntime }

func (r *countRecognizer) Decode(samples []float32, _ int) (asr.Result, error) {
	r.runtime.mu.Lock()
	r.runtime.sizes = append(r.runtime.sizes, len(samples))
	n := len(r.runtime.sizes)
	fail := r.runtime.failAt == n
	r.runtime.mu.Unlock()
	if fail {
		return asr.Result{}, errors.New("decode failed")
	}
	return asr.Result{Text: "段", Language: "zh"}, nil
}
func (r *countRecognizer) Close() {}

func (r *countRuntime) lengths() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.sizes...)
}

type scriptVoice struct {
	mu             sync.Mutex
	profile        *voiceprint.Profile
	relaxed        []bool
	seen           *voiceprint.Profile
	hold           chan struct{}
	release        chan struct{}
	splits         bool
	rescue         bool
	frozenRef      []float32
	frozenComplete bool
}

func (s *scriptVoice) Snapshot() *voiceprint.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile == nil {
		return nil
	}
	clone := *s.profile
	clone.Embeddings = append([][]float32{}, s.profile.Embeddings...)
	return &clone
}
func (s *scriptVoice) Enrolled() bool { return s.Snapshot() != nil }
func (s *scriptVoice) Match(*voiceprint.Profile, []float32, int, *voiceprint.Reference) (string, float32) {
	return "me", 1
}
func (s *scriptVoice) MatchSpansRelaxed(profile *voiceprint.Profile, samples []float32, _ int, ref *voiceprint.Reference, relax bool) []voiceprint.IdentitySpan {
	s.mu.Lock()
	s.relaxed = append(s.relaxed, relax)
	if s.seen == nil {
		s.seen = profile
	}
	s.frozenComplete = ref != nil && ref.Complete
	s.frozenRef = nil
	if ref != nil {
		s.frozenRef = append([]float32(nil), ref.Samples...)
	}
	hold, release := s.hold, s.release
	s.hold = nil
	splits, rescue := s.splits, s.rescue
	s.mu.Unlock()
	if hold != nil {
		close(hold)
		<-release
	}
	n := len(samples)
	if splits && n >= 3 {
		cut := n / 3
		return []voiceprint.IdentitySpan{
			{Start: 0, End: cut, Identity: "me", Score: 0.9, Anchor: true},
			{Start: cut, End: 2 * cut, Identity: "not_me", Score: 0.1},
			{Start: 2 * cut, End: n, Identity: "me", Score: 0.8, Anchor: true},
		}
	}
	identity, anchor := "not_me", false
	if len(samples) > 0 && samples[0] == 1 {
		identity, anchor = "me", true
	}
	if len(samples) > 0 && samples[0] == 2 && rescue && relax {
		identity = "me"
	}
	return []voiceprint.IdentitySpan{{Start: 0, End: n, Identity: identity, Score: 0.4, Anchor: anchor}}
}

func installVoice(e *Engine, voice voiceMatcher) {
	e.mu.Lock()
	e.voice = voice
	e.mu.Unlock()
}

type spanCapture struct{ fakeCapture }

func (*spanCapture) Devices() ([]protocol.DeviceInfo, error) {
	// Identity tests bind to mic-1. Report it as present so the independent
	// initial device poll cannot treat the fixture as a real disconnection.
	return []protocol.DeviceInfo{
		{ID: "default", IsDefault: true, Label: "Default Test Microphone"},
		{ID: "mic-1", Label: "Span Test Microphone"},
	}, nil
}

func spanEngine(t *testing.T, rt asr.Runtime) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	settings := protocol.DefaultSettings(dir)
	settings.Recording.AutoSwitchDevice = false
	settings.Recording.EchoCancellation = false
	engine := New(settings, &spanCapture{}, &fakeAssets{}, rt, nil, nil)
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	return engine, dir
}

func stopSoon(t *testing.T, e *Engine) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- e.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop deadlocked")
	}
}

func TestSpanJobKeepsOneVADInOneDecode(t *testing.T) {
	rt := &countRuntime{}
	e, root := spanEngine(t, rt)
	installVoice(e, &scriptVoice{profile: acceptedLegacy(), splits: true})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var start time.Time
	samples := make([]float32, 48000)
	if err := e.call(func() error {
		start = e.segment.start
		return e.appendSpeech(&e.audioStream, asr.Speech{Samples: samples})
	}); err != nil {
		t.Fatal(err)
	}
	stopSoon(t, e)
	items := History([]string{root})
	if len(items) != 1 || len(items[0].TranscriptChunks) != 1 || items[0].TextPreview != "段" {
		t.Fatalf("split transcript: %+v", items)
	}
	chunk := items[0].TranscriptChunks[0]
	if chunk.VoiceIdentity != "" || chunk.VoiceMatch != "partial" {
		t.Fatalf("mixed utterance was promoted: %+v", chunk)
	}
	gotStart, err1 := time.Parse(time.RFC3339Nano, chunk.StartedAt)
	gotEnd, err2 := time.Parse(time.RFC3339Nano, chunk.EndedAt)
	wantEnd := start.Add(time.Duration(len(samples)) * time.Second / audio.SampleRate)
	if err1 != nil || err2 != nil || !gotStart.Equal(start) || !gotEnd.Equal(wantEnd) {
		t.Fatalf("utterance clock %s %s", chunk.StartedAt, chunk.EndedAt)
	}
	if sizes := rt.lengths(); len(sizes) != 1 || sizes[0] != len(samples) {
		t.Fatalf("decode sizes %v", sizes)
	}
	raw := readRecordingJSON(t, root)
	if raw["capture_device_id"] == nil || raw["echo_cancellation"] != false {
		t.Fatalf("capture context missing: %v", raw)
	}
}

func TestSpanFailureStillArchivesAudio(t *testing.T) {
	e, root := spanEngine(t, &countRuntime{failAt: 1})
	installVoice(e, &scriptVoice{profile: ownerProfile(), splits: true})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		return e.appendSpeech(&e.audioStream, asr.Speech{Samples: make([]float32, 48000)})
	}); err != nil {
		t.Fatal(err)
	}
	stopSoon(t, e)
	files, err := AudioFiles([]string{root})
	if err != nil || len(files) != 1 {
		t.Fatal("audio was not archived", files, err)
	}
	if samples := archivedSamples(t, files[0]); len(samples) != 48000 {
		t.Fatalf("original audio was truncated: %d samples", len(samples))
	}
	raw, err := os.ReadFile(strings.TrimSuffix(files[0], filepath.Ext(files[0])) + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil || metadata["transcription_incomplete"] != true {
		t.Fatalf("failure was not marked incomplete: %s", raw)
	}
}

func TestSpanKeepsShortTail(t *testing.T) {
	e, root := spanEngine(t, &countRuntime{})
	installVoice(e, &scriptVoice{profile: acceptedLegacy()})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var start time.Time
	samples := make([]float32, 16100)
	samples[0] = 1
	if err := e.call(func() error {
		start = e.segment.start
		return e.appendSpeech(&e.audioStream, asr.Speech{Samples: samples})
	}); err != nil {
		t.Fatal(err)
	}
	stopSoon(t, e)
	items := History([]string{root})
	if len(items) != 1 || len(items[0].TranscriptChunks) != 1 || items[0].TranscriptChunks[0].VoiceIdentity != "me" {
		t.Fatalf("tail transcript: %+v", items)
	}
	got, err := time.Parse(time.RFC3339Nano, items[0].TranscriptChunks[0].EndedAt)
	want := start.Add(time.Duration(len(samples)) * time.Second / audio.SampleRate)
	if err != nil || !got.Equal(want) {
		t.Fatalf("tail end %s want %s", items[0].TranscriptChunks[0].EndedAt, want.Format(time.RFC3339Nano))
	}
	if sizes := e.runtime.(*countRuntime).lengths(); len(sizes) != 1 || sizes[0] != 16100 {
		t.Fatalf("tail decode %v", sizes)
	}
}

func TestRescuedMeDoesNotPropagate(t *testing.T) {
	rt := &countRuntime{}
	e, root := spanEngine(t, rt)
	voice := &scriptVoice{profile: acceptedLegacy(), rescue: true}
	installVoice(e, voice)
	e.mu.Lock()
	e.activeDevice = "mic-1"
	e.mu.Unlock()
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var base time.Time
	if err := e.call(func() error {
		base = e.segment.start
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	speak := func(offset int, mark float32) {
		t.Helper()
		samples := make([]float32, 16000)
		samples[0] = mark
		start := base.Add(time.Duration(offset) * time.Second / audio.SampleRate)
		e.echoRef.AppendAt(start, make([]float32, len(samples)))
		if err := e.call(func() error {
			return e.appendSpeech(&e.audioStream, asr.Speech{Start: offset, Samples: samples})
		}); err != nil {
			t.Fatal(err)
		}
	}
	speak(0, 1)
	speak(24000, 2)
	speak(48000, 2)
	stopSoon(t, e)
	voice.mu.Lock()
	flags := append([]bool(nil), voice.relaxed...)
	voice.mu.Unlock()
	if len(flags) != 3 || flags[0] || !flags[1] || flags[2] {
		t.Fatalf("relax chain = %v", flags)
	}
	items := History([]string{root})
	if len(items) != 1 || len(items[0].TranscriptChunks) != 3 {
		t.Fatalf("rescued transcript: %+v", items)
	}
	if items[0].TranscriptChunks[0].VoiceIdentity != "me" || items[0].TranscriptChunks[1].VoiceIdentity != "me" || items[0].TranscriptChunks[2].VoiceIdentity != "" {
		t.Fatalf("propagation badges: %+v", items[0].TranscriptChunks)
	}
}

func TestNilReferenceDoesNotRelax(t *testing.T) {
	e, _ := spanEngine(t, &countRuntime{})
	voice := &scriptVoice{profile: acceptedLegacy(), rescue: true}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	speak := func(offset int) {
		t.Helper()
		samples := make([]float32, 16000)
		samples[0] = 1
		if err := e.call(func() error {
			return e.appendSpeech(&e.audioStream, asr.Speech{Start: offset, Samples: samples})
		}); err != nil {
			t.Fatal(err)
		}
	}
	speak(0)
	speak(8000)
	stopSoon(t, e)
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if len(voice.relaxed) != 2 || voice.relaxed[0] || voice.relaxed[1] {
		t.Fatalf("nil reference relaxed: %v", voice.relaxed)
	}
}

func TestBatchSpanContextAndLegacy(t *testing.T) {
	e, _, assets, root := testEngine(t, false, &fakeRuntime{})
	voice := &scriptVoice{profile: calibratedProfile("mic-1", false), splits: true}
	installVoice(e, voice)
	start := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	samples := make([]float32, 48000)
	writeMic(t, root, "daybee_20261012_010000_microphone.wav", samples, start, map[string]any{
		"capture_device_id": "mic-2", "echo_cancellation": false,
	})
	assets.read = func(context.Context, string) ([]float32, int, error) { return samples, audio.SampleRate, nil }
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	mic := onlyMic(t, root)
	if mic.TextPreview != "固定测试文本" || len(mic.TranscriptChunks) != 1 || mic.TranscriptChunks[0].VoiceIdentity != "" || mic.TranscriptChunks[0].VoiceMatch != "" {
		t.Fatalf("mismatched batch kept a badge: %+v", mic)
	}
	if mic.TranscriptChunks[0].StartedAt != start.Format(time.RFC3339Nano) {
		t.Fatalf("batch time %s", mic.TranscriptChunks[0].StartedAt)
	}

	legacyRoot := t.TempDir()
	e2, _, assets2, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e2, &scriptVoice{profile: ownerProfile(), splits: true})
	called := false
	previous := profileCompatible
	profileCompatible = func(*voiceprint.Profile, voiceprint.EnrollmentContext) bool {
		called = true
		return false
	}
	t.Cleanup(func() { profileCompatible = previous })
	writeMic(t, legacyRoot, "daybee_20261012_020000_microphone.wav", samples, start, nil)
	assets2.read = func(context.Context, string) ([]float32, int, error) { return samples, audio.SampleRate, nil }
	if err := e2.Transcribe(legacyRoot); err != nil {
		t.Fatal(err)
	}
	legacy := onlyMic(t, legacyRoot)
	if called || len(legacy.TranscriptChunks) != 1 || legacy.TranscriptChunks[0].VoiceIdentity != "" || legacy.TranscriptChunks[0].VoiceMatch != "partial" {
		t.Fatalf("legacy context guessed: called=%v %+v", called, legacy.TranscriptChunks)
	}

	v3Root := t.TempDir()
	e3, _, assets3, _ := testEngine(t, false, &fakeRuntime{})
	installVoice(e3, &scriptVoice{profile: calibratedProfile("mic-1", false), splits: true})
	called = false
	writeMic(t, v3Root, "daybee_20261012_030000_microphone.wav", samples, start, nil)
	assets3.read = func(context.Context, string) ([]float32, int, error) { return samples, audio.SampleRate, nil }
	if err := e3.Transcribe(v3Root); err != nil {
		t.Fatal(err)
	}
	bare := onlyMic(t, v3Root)
	if called || bare.TextPreview == "" || len(bare.TranscriptChunks) != 1 || bare.TranscriptChunks[0].VoiceIdentity != "" || bare.TranscriptChunks[0].VoiceMatch != "" {
		t.Fatalf("v3 without context guessed a match: called=%v %+v", called, bare)
	}
}

func acceptedLegacy() *voiceprint.Profile {
	vector := make([]float32, 192)
	vector[0] = 1
	return &voiceprint.Profile{Version: 1, Model: voiceprint.ModelName, Embeddings: [][]float32{vector}, CreatedAt: "frozen"}
}

func calibratedProfile(device string, echo bool) *voiceprint.Profile {
	vector := make([]float32, 192)
	vector[0] = 1
	_, meta, _ := voiceprint.LoadCohort()
	return &voiceprint.Profile{Version: 3, Model: voiceprint.ModelName, Embeddings: [][]float32{vector}, CreatedAt: "frozen",
		Device: device, EchoCancellation: echo, Calibration: &voiceprint.Calibration{TLow: 0.2, THigh: 0.8, Model: voiceprint.ModelName, CohortID: meta.BlobSHA256}}
}

func writeMic(t *testing.T, root, name string, samples []float32, start time.Time, extra map[string]any) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("mic"), 0600); err != nil {
		t.Fatal(err)
	}
	end := start.Add(time.Duration(len(samples)) * time.Second / audio.SampleRate)
	payload := map[string]any{
		"audio_path": path, "audio_source": protocol.SourceMicrophone, "segment_start_time": start.Format(time.RFC3339Nano),
		"recording_session_id": "session-a", "captured_ranges": []protocol.RecordingTimeRange{{
			StartedAt: start.Format(time.RFC3339Nano), EndedAt: end.Format(time.RFC3339Nano),
		}},
	}
	for key, value := range extra {
		payload[key] = value
	}
	if err := config.WriteJSON(stringsTrim(path), payload); err != nil {
		t.Fatal(err)
	}
}

func stringsTrim(path string) string {
	return path[:len(path)-len(filepath.Ext(path))] + ".recording.json"
}

func onlyMic(t *testing.T, root string) protocol.RecordingHistoryItem {
	t.Helper()
	items := History([]string{root})
	if len(items) != 1 {
		t.Fatalf("history %+v", items)
	}
	return items[0]
}

func readRecordingJSON(t *testing.T, root string) map[string]any {
	t.Helper()
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".recording.json") {
			return walkErr
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil || len(matches) != 1 {
		t.Fatal(matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		t.Fatal("recording json")
	}
	return payload
}
