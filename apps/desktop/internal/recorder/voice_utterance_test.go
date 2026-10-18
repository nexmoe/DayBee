package recorder

import (
	"math"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func TestUtteranceVerdict(t *testing.T) {
	spans := func(labels ...string) []voiceprint.IdentitySpan {
		out := make([]voiceprint.IdentitySpan, len(labels))
		for i, label := range labels {
			out[i] = voiceprint.IdentitySpan{Start: i * 16000, End: (i + 1) * 16000, Identity: label, Score: float32(i+1) / 10, Anchor: label == "me" && i%2 == 0}
		}
		return out
	}
	n := 16000
	identity, match, score, seed := utteranceVerdict(spans("me"), n, true)
	if identity != "me" || match != "" || !seed || math.Abs(score-0.1) > 1e-6 {
		t.Fatalf("confirmed me: %s %s %.8f %v", identity, match, score, seed)
	}
	identity, match, score, seed = utteranceVerdict([]voiceprint.IdentitySpan{
		{Start: 0, End: 16000, Identity: "me", Score: 0.9, Anchor: false},
	}, n, true)
	if identity != "me" || match != "" || seed {
		t.Fatalf("unanchored me still labeled but not seeded: %s %s %v", identity, match, seed)
	}
	identity, match, _, seed = utteranceVerdict(spans("me", "not_me", "uncertain", "me"), 4*n, true)
	if identity != "" || match != "partial" || seed {
		t.Fatalf("mixed confidence: %s %s %v", identity, match, seed)
	}
	identity, match, _, seed = utteranceVerdict(spans("not_me", "uncertain"), 2*n, true)
	if identity != "" || match != "" || seed {
		t.Fatalf("no me: %s %s %v", identity, match, seed)
	}
	identity, match, _, seed = utteranceVerdict(spans("me"), n, false)
	if identity != "" || match != "" || seed {
		t.Fatalf("incompatible device: %s %s %v", identity, match, seed)
	}
	for _, bad := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		identity, match, score, seed = utteranceVerdict([]voiceprint.IdentitySpan{
			{Start: 0, End: 8000, Identity: "me", Score: bad, Anchor: true},
			{Start: 8000, End: 16000, Identity: "me", Score: 0.5, Anchor: true},
		}, n, true)
		if identity != "" || match != "" || seed || score != 0 {
			t.Fatalf("non-finite score stayed usable: %s %s %.3f %v", identity, match, score, seed)
		}
	}
	meOnly := []voiceprint.IdentitySpan{{Start: 0, End: n, Identity: "me", Score: 0.9, Anchor: true}}
	for _, malformed := range [][]voiceprint.IdentitySpan{
		{{Start: 0, End: 8000, Identity: "me", Score: 0.9, Anchor: true}},
		{{Start: 100, End: n, Identity: "me", Score: 0.9, Anchor: true}},
		{{Start: 0, End: 8000, Identity: "me", Score: 0.9, Anchor: true}, {Start: 9000, End: n, Identity: "me", Score: 0.9, Anchor: true}},
		{{Start: 0, End: 9000, Identity: "me", Score: 0.9, Anchor: true}, {Start: 8000, End: n, Identity: "me", Score: 0.9, Anchor: true}},
		append([]voiceprint.IdentitySpan{{Start: -1, End: 10, Identity: "not_me", Score: 0.1}}, meOnly...),
	} {
		identity, match, score, seed = utteranceVerdict(malformed, n, true)
		if identity != "" || match != "" || seed || score != 0 {
			t.Fatalf("malformed cover became me: %+v -> %s %s %.3f %v", malformed, identity, match, score, seed)
		}
	}
	_, _, score, _ = utteranceVerdict([]voiceprint.IdentitySpan{
		{Start: 0, End: 1000, Identity: "me", Score: 0.2, Anchor: true},
		{Start: 1000, End: 3000, Identity: "not_me", Score: 0.8},
	}, 3000, true)
	if math.Abs(score-(0.2*1000+0.8*2000)/3000) > 1e-5 {
		t.Fatalf("weighted score %.8f", score)
	}
}
