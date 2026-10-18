package recorder

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type utteranceNativeRuntime struct {
	fakeRuntime
	model string
	mu    sync.Mutex
	sizes []int
}

func (r *utteranceNativeRuntime) NewRecognizer(string) (asr.Recognizer, error) {
	native, err := (asr.Sherpa{}).NewRecognizer(r.model)
	if err != nil {
		return nil, err
	}
	return &utteranceNativeRecognizer{Recognizer: native, runtime: r}, nil
}

type utteranceNativeRecognizer struct {
	asr.Recognizer
	runtime *utteranceNativeRuntime
}

func (r *utteranceNativeRecognizer) Decode(samples []float32, rate int) (asr.Result, error) {
	r.runtime.mu.Lock()
	r.runtime.sizes = append(r.runtime.sizes, len(samples))
	r.runtime.mu.Unlock()
	return r.Recognizer.Decode(samples, rate)
}

// Runs the actual recorder job with two local native models and public audio.
// No real capture, user profile, or user recording is accessed.
func TestNativeUtteranceASRContext(t *testing.T) {
	model, wav, asrModel := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_MODEL"), os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_WAV"), os.Getenv("DAYBEE_TEST_MODEL_DIR")
	if model == "" || wav == "" || asrModel == "" {
		t.Skip("set voiceprint model, public WAV directory and DAYBEE_TEST_MODEL_DIR")
	}
	read := func(name string) []float32 {
		t.Helper()
		pcm, rate, err := (asr.Sherpa{}).ReadWave(filepath.Join(wav, name))
		if err != nil || rate != audio.SampleRate || len(pcm) == 0 {
			t.Fatalf("%s: rate=%d, %v", name, rate, err)
		}
		return pcm
	}
	m, err := voiceprint.New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	enrollment := read("SSB0544_247.wav")
	if len(enrollment) < 8*audio.SampleRate {
		t.Fatal("short enrollment")
	}
	vectors, _, err := m.EmbedWindows(enrollment[:8*audio.SampleRate], audio.SampleRate)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(vectors); err != nil {
		t.Fatal(err)
	}
	var me []float32
	for _, name := range []string{"SSB0544_443.wav", "SSB0544_441.wav", "SSB0544_411.wav", "SSB0544_406.wav", "SSB0544_399.wav", "SSB0544_330.wav"} {
		me = append(me, read(name)...)
	}
	other := read("SSB0700_265.wav")
	if len(me) < 4*audio.SampleRate || len(other) < 4*audio.SampleRate {
		t.Fatal("short held-out speech")
	}
	sequence := append(append(append([]float32{}, me[:4*audio.SampleRate]...), other[:4*audio.SampleRate]...), me[:4*audio.SampleRate]...)
	video := append([]float32(nil), me...)
	for i := range video {
		video[i] *= 0.5
	}
	cases := []struct {
		name     string
		pcm, ref []float32
	}{
		{"clean", me, make([]float32, len(me))},
		{"me-other-me", sequence, make([]float32, len(sequence))},
		{"other", other, make([]float32, len(other))},
		{"own-video", video, me},
	}
	rt := &utteranceNativeRuntime{model: asrModel}
	e, root := spanEngine(t, rt)
	installVoice(e, m)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	var expected []int
	for i, tc := range cases {
		expected = append(expected, len(tc.pcm))
		spans := m.MatchSpans(m.Snapshot(), tc.pcm, audio.SampleRate, &voiceprint.Reference{Samples: tc.ref, Complete: true})
		if tc.name == "me-other-me" && len(spans) <= 1 {
			t.Fatalf("fixture does not exercise confidence boundaries: %+v", spans)
		}
		t.Logf("%s: %.3fs, %d confidence spans; must remain one decode", tc.name, float64(len(tc.pcm))/audio.SampleRate, len(spans))
		if err := e.call(func() error {
			offset := i * 20 * audio.SampleRate
			start := atSample(e.segment.start, offset, audio.SampleRate)
			e.echoRef.AppendAt(start, tc.ref)
			return e.appendSpeech(&e.audioStream, asr.Speech{Start: offset, Samples: tc.pcm})
		}); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	sizes := append([]int(nil), rt.sizes...)
	rt.mu.Unlock()
	if !reflect.DeepEqual(sizes, expected) {
		t.Fatalf("native ASR lost utterance context: sizes=%v want=%v", sizes, expected)
	}
	item := onlyMic(t, root)
	if len(item.TranscriptChunks) != len(cases) {
		t.Fatalf("chunks=%d want=%d", len(item.TranscriptChunks), len(cases))
	}
	for i, chunk := range item.TranscriptChunks {
		if chunk.Text == "" {
			t.Fatalf("%s lost transcription", cases[i].name)
		}
		if i > 0 && chunk.VoiceIdentity != "" {
			t.Fatalf("%s promoted mixed/other audio to me", cases[i].name)
		}
		if i == 0 && (chunk.VoiceIdentity != "me" || chunk.VoiceMatch != "") {
			t.Fatalf("clean fixture lost its confirmed identity: %+v", chunk)
		}
		if i == 1 && chunk.VoiceMatch != "partial" {
			t.Fatalf("mixed fixture lost its partial evidence: %+v", chunk)
		}
		if i > 1 && chunk.VoiceMatch != "" {
			t.Fatalf("%s incorrectly gained partial evidence: %+v", cases[i].name, chunk)
		}
		t.Logf("%s: one complete chunk, identity=%q, text characters=%d", cases[i].name, chunk.VoiceIdentity, len([]rune(chunk.Text)))
	}
	t.Logf("drained native jobs in %s (not a queue or RSS benchmark)", time.Since(started))
}
