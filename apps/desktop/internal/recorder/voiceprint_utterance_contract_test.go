package recorder

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type confidenceVoice struct {
	scriptVoice
	labels []string
}

func (v *confidenceVoice) MatchSpansRelaxed(_ *voiceprint.Profile, samples []float32, _ int, _ *voiceprint.Reference, relax bool) []voiceprint.IdentitySpan {
	v.mu.Lock()
	v.relaxed = append(v.relaxed, relax)
	v.mu.Unlock()
	parts := make([]voiceprint.IdentitySpan, len(v.labels))
	for i, identity := range v.labels {
		parts[i] = voiceprint.IdentitySpan{Start: i * len(samples) / len(parts), End: (i + 1) * len(samples) / len(parts), Identity: identity, Score: 0.6, Anchor: identity == "me"}
	}
	return parts
}

func TestPartialUtteranceDoesNotRelaxNextCapture(t *testing.T) {
	e, _ := spanEngine(t, &countRuntime{})
	voice := &confidenceVoice{scriptVoice: scriptVoice{profile: acceptedLegacy()}, labels: []string{"me", "not_me", "me"}}
	installVoice(e, voice)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.call(func() error {
		for _, offset := range []int{0, 4 * audio.SampleRate} {
			samples := make([]float32, 3*audio.SampleRate)
			start := atSample(e.segment.start, offset, audio.SampleRate)
			e.echoRef.AppendAt(start, make([]float32, len(samples)))
			if err := e.appendSpeech(&e.audioStream, asr.Speech{Start: offset, Samples: samples}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stopSoon(t, e)
	voice.mu.Lock()
	flags := append([]bool(nil), voice.relaxed...)
	voice.mu.Unlock()
	if !reflect.DeepEqual(flags, []bool{false, false}) {
		t.Fatalf("partial matching propagated a me anchor: %v", flags)
	}
}

// A verifier's confidence transitions do not establish speaker turns. The
// recognizer must receive a complete VAD utterance, including uncertain audio.
func TestConfidenceChangesKeepOneUtterance(t *testing.T) {
	for _, tc := range []struct {
		name            string
		labels          []string
		identity, match string
	}{
		{"confirmed", []string{"me"}, "me", ""},
		{"confidence dip", []string{"me", "not_me", "uncertain", "me"}, "", "partial"},
		{"mixed speakers remain unconfirmed", []string{"me", "not_me", "me"}, "", "partial"},
		{"unconfirmed", []string{"not_me", "uncertain"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &countRuntime{}
			e, root := spanEngine(t, rt)
			installVoice(e, &confidenceVoice{scriptVoice: scriptVoice{profile: acceptedLegacy()}, labels: tc.labels})
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			var start time.Time
			samples := make([]float32, 7*audio.SampleRate+9968)
			if err := e.call(func() error {
				start = e.segment.start
				return e.appendSpeech(&e.audioStream, asr.Speech{Samples: samples})
			}); err != nil {
				t.Fatal(err)
			}
			stopSoon(t, e)
			if sizes := rt.lengths(); !reflect.DeepEqual(sizes, []int{len(samples)}) {
				t.Fatalf("confidence changes cut ASR context: decode lengths %v, want [%d]", sizes, len(samples))
			}
			item := onlyMic(t, root)
			if len(item.TranscriptChunks) != 1 {
				t.Fatalf("one utterance produced %d chunks", len(item.TranscriptChunks))
			}
			chunk := item.TranscriptChunks[0]
			if chunk.VoiceIdentity != tc.identity {
				t.Fatalf("mixed confidence became a uniform identity: %+v", chunk)
			}
			raw, err := json.Marshal(chunk)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			match, _ := fields["voiceMatch"].(string)
			if match != tc.match {
				t.Fatalf("partial evidence was lost: %s", raw)
			}
			end := atSample(start, len(samples), audio.SampleRate).Format(time.RFC3339Nano)
			if chunk.StartedAt != start.Format(time.RFC3339Nano) || chunk.EndedAt != end {
				t.Fatalf("utterance clock truncated: %+v", chunk)
			}
		})
	}
}
