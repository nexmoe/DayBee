package recorder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type fakeVoice struct {
	mu      sync.Mutex
	profile *voiceprint.Profile
	calls   int
	panic   bool
	score   float32
	match   func(*voiceprint.Profile, []float32) string
}

func (f *fakeVoice) Snapshot() *voiceprint.Profile {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.profile == nil || len(f.profile.Embeddings) == 0 {
		return nil
	}
	clone := *f.profile
	clone.Embeddings = append([][]float32{}, f.profile.Embeddings...)
	return &clone
}
func (f *fakeVoice) Enrolled() bool { return f.Snapshot() != nil }
func (f *fakeVoice) Match(profile *voiceprint.Profile, samples []float32, _ int, _ *voiceprint.Reference) (string, float32) {
	f.mu.Lock()
	f.calls++
	panicNow := f.panic
	match := f.match
	score := f.score
	f.mu.Unlock()
	if panicNow {
		panic("voiceprint match failed")
	}
	if match != nil {
		return match(profile, samples), score
	}
	return "me", score
}

func ownerProfile() *voiceprint.Profile {
	return &voiceprint.Profile{Version: 1, Model: voiceprint.ModelName, Embeddings: [][]float32{{1, 0}}, CreatedAt: "frozen"}
}

func useVoice(e *Engine, voice *fakeVoice) {
	e.mu.Lock()
	e.voice = voice
	e.mu.Unlock()
}

func recordOne(t *testing.T, e *Engine, capture *fakeCapture, source protocol.AudioSource) []protocol.RecordingHistoryItem {
	t.Helper()
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	packet := audio.Packet{Samples: make([]float32, audio.WindowSize), DeviceLabel: "Test Mic", Source: source}
	if err := e.call(func() error { return e.consume(packet) }); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	_ = capture
	return History([]string{e.Settings().Recording.OutputDir})
}

func TestVoiceprintLabelsMicrophoneWithoutDroppingText(t *testing.T) {
	cases := []struct {
		name      string
		missing   bool
		source    protocol.AudioSource
		system    bool
		panic     bool
		match     func(*voiceprint.Profile, []float32) string
		wantID    string
		wantScore float64
		wantCall  bool
	}{
		{name: "me-by-default", source: protocol.SourceMicrophone, match: func(*voiceprint.Profile, []float32) string { return "me" }, wantID: "me", wantScore: 0.5, wantCall: true},
		{name: "unknown", source: protocol.SourceMicrophone, match: func(*voiceprint.Profile, []float32) string { return "" }, wantScore: 0.25, wantCall: true},
		{name: "panic", source: protocol.SourceMicrophone, panic: true, wantCall: true},
		{name: "not-enrolled", missing: true, source: protocol.SourceMicrophone},
		{name: "system", source: protocol.SourceSystem, system: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, capture, _, root := testEngine(t, false, &fakeRuntime{})
			voice := &fakeVoice{profile: ownerProfile(), panic: tc.panic, match: tc.match, score: float32(tc.wantScore)}
			if tc.missing {
				voice.profile = nil
			}
			settings := e.Settings()
			settings.Recording.RecordSystemAudio = tc.system
			if err := e.Apply(settings); err != nil {
				t.Fatal(err)
			}
			e.mu.Lock()
			e.voice = voice
			e.mu.Unlock()
			items := recordOne(t, e, capture, tc.source)
			if len(items) != 1 || items[0].TextPreview != "固定测试文本" || len(items[0].TranscriptChunks) != 1 {
				t.Fatalf("speech text was dropped: %+v", items)
			}
			if items[0].TranscriptChunks[0].VoiceIdentity != tc.wantID || items[0].TranscriptChunks[0].VoiceScore != tc.wantScore {
				t.Fatalf("identity = %q score = %v", items[0].TranscriptChunks[0].VoiceIdentity, items[0].TranscriptChunks[0].VoiceScore)
			}
			if voice.calls > 0 != tc.wantCall {
				t.Fatalf("match calls = %d", voice.calls)
			}
			raw, err := os.ReadFile(strings.TrimSuffix(items[0].AudioPath, filepath.Ext(items[0].AudioPath)) + ".json")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "transcription_incomplete") || strings.Contains(string(raw), "embeddings") {
				t.Fatalf("label failure changed transcript metadata: %s", raw)
			}
			files, err := AudioFiles([]string{root})
			if err != nil || len(files) != 1 {
				t.Fatal("audio was dropped", files, err)
			}
		})
	}
}

func TestVoiceprintFreezesProfileAtSubmission(t *testing.T) {
	runtime := &fakeRuntime{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	e, _, _, _ := testEngine(t, false, runtime)
	voice := &fakeVoice{profile: ownerProfile(), match: func(profile *voiceprint.Profile, _ []float32) string {
		if profile != nil && profile.CreatedAt == "frozen" {
			return "me"
		}
		return ""
	}}
	useVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		return e.consume(audio.Packet{Samples: make([]float32, audio.WindowSize), DeviceLabel: "Test Mic"})
	}); err != nil {
		t.Fatal(err)
	}
	<-runtime.entered
	voice.mu.Lock()
	voice.profile = nil
	voice.mu.Unlock()
	close(runtime.gate)
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	items := History([]string{e.Settings().Recording.OutputDir})
	if len(items) != 1 || items[0].TextPreview != "固定测试文本" || len(items[0].TranscriptChunks) != 1 || items[0].TranscriptChunks[0].VoiceIdentity != "me" {
		t.Fatalf("frozen profile was not labeled: %+v", items)
	}
}

func TestHistoryLoadsChunksWithAndWithoutVoiceIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "daybee_20261011_120000.wav")
	if err := os.WriteFile(path, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	chunks := []protocol.TranscriptChunk{
		{Text: "旧文本", StartedAt: "2026-10-11T12:00:01Z", EndedAt: "2026-10-11T12:00:02Z"},
		{Text: "新文本", StartedAt: "2026-10-11T12:00:03Z", EndedAt: "2026-10-11T12:00:04Z", VoiceIdentity: "me"},
	}
	if err := config.WriteJSON(strings.TrimSuffix(path, ".wav")+".json", map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-11T12:00:00Z", "status": "ok", "text": "旧文本\n新文本", "transcript_chunks": chunks,
	}); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root})
	if len(items) != 1 || items[0].TextPreview != "旧文本\n新文本" || len(items[0].TranscriptChunks) != 2 {
		t.Fatalf("history dropped mixed chunks: %+v", items)
	}
	if items[0].TranscriptChunks[0].VoiceIdentity != "" || items[0].TranscriptChunks[1].VoiceIdentity != "me" {
		t.Fatalf("identity was not preserved: %+v", items[0].TranscriptChunks)
	}
}

func TestBatchLabelsEveryMicrophonePiece(t *testing.T) {
	e, _, assets, root := testEngine(t, false, &fakeRuntime{})
	voice := &fakeVoice{profile: ownerProfile(), match: func(_ *voiceprint.Profile, samples []float32) string {
		if len(samples) > 0 && samples[0] == 1 {
			return "me"
		}
		return ""
	}}
	useVoice(e, voice)
	start := time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)
	ranges := []protocol.RecordingTimeRange{
		{StartedAt: start.Format(time.RFC3339Nano), EndedAt: start.Add(100 * time.Millisecond).Format(time.RFC3339Nano)},
		{StartedAt: start.Add(100 * time.Millisecond).Format(time.RFC3339Nano), EndedAt: start.Add(200 * time.Millisecond).Format(time.RFC3339Nano)},
	}
	samples := make([]float32, 3200)
	samples[0] = 1
	path := filepath.Join(root, "mic.wav")
	if err := os.WriteFile(path, []byte("mic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteJSON(strings.TrimSuffix(path, ".wav")+".recording.json", map[string]any{
		"audio_path": path, "audio_source": protocol.SourceMicrophone, "segment_start_time": start.Format(time.RFC3339Nano), "captured_ranges": ranges,
	}); err != nil {
		t.Fatal(err)
	}
	systemPath := filepath.Join(root, "system.wav")
	if err := os.WriteFile(systemPath, []byte("system"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteJSON(strings.TrimSuffix(systemPath, ".wav")+".recording.json", map[string]any{
		"audio_path": systemPath, "audio_source": protocol.SourceSystem, "segment_start_time": start.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	assets.read = func(_ context.Context, file string) ([]float32, int, error) {
		if file == systemPath {
			return []float32{0.2}, audio.SampleRate, nil
		}
		return samples, audio.SampleRate, nil
	}
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root})
	if len(items) != 2 {
		t.Fatalf("batch dropped a file: %+v", items)
	}
	var mic, system protocol.RecordingHistoryItem
	for _, item := range items {
		if item.AudioSource == protocol.SourceSystem {
			system = item
		} else {
			mic = item
		}
	}
	if mic.TextPreview != "固定测试文本\n固定测试文本" || len(mic.TranscriptChunks) != 2 {
		t.Fatalf("microphone pieces were not all kept: %+v", mic)
	}
	if mic.TranscriptChunks[0].VoiceIdentity != "me" || mic.TranscriptChunks[1].VoiceIdentity != "" {
		t.Fatalf("batch labeled the wrong pieces: %+v", mic.TranscriptChunks)
	}
	if len(mic.CapturedRanges) != 2 || mic.CapturedRanges[0].StartedAt != ranges[0].StartedAt {
		t.Fatalf("captured ranges were replaced: %+v", mic.CapturedRanges)
	}
	if system.TextPreview != "固定测试文本" || len(system.TranscriptChunks) != 0 || system.AudioSource != protocol.SourceSystem {
		t.Fatalf("system file was labeled or split: %+v", system)
	}
	if voice.calls != 2 {
		t.Fatalf("system or extra audio was matched: %d", voice.calls)
	}
}
