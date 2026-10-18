package voiceprint

import (
	"math/rand/v2"
	"testing"
)

// Independently check that boundary protection never expands the set of me
// samples, or damages coverage, across varied scores and window supports.
func TestBoundaryProtectionOnlyRemovesMe(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261012, 17))
	p := &Profile{Version: 2, Model: ModelName}
	threshold := Threshold(ModelName)
	for trial := 0; trial < 400; trial++ {
		n := 32000 + rng.IntN(240000)
		var wins []WindowScore
		var scores []float32
		var echo []bool
		for start := 0; start+12800 <= n; start += 8000 {
			end := start + 12800 + rng.IntN(19201)
			if end > n {
				end = n
			}
			wins = append(wins, WindowScore{Start: start, End: end})
			scores = append(scores, []float32{asNormMin, 0.2, 0.53, 0.6, 0.95}[rng.IntN(5)])
			echo = append(echo, rng.IntN(8) == 0)
		}
		before := IdentitySpans(wins, scores, echo, n, 16000, threshold, threshold)
		before = reviewMeSpans(p, wins, scores, echo, before, n, threshold)
		after := guardBoundaryOverlap(p, wins, scores, echo, before, n, threshold)
		end := 0
		for _, span := range after {
			if span.Start != end || span.End <= span.Start || span.End > n || !finite32(span.Score) {
				t.Fatalf("trial %d invalid coverage: %+v", trial, after)
			}
			end = span.End
			for _, original := range before {
				if span.End <= original.Start || span.Start >= original.End {
					continue
				}
				if span.Identity == spanMe && original.Identity != spanMe {
					t.Fatalf("trial %d introduced me: before=%+v after=%+v", trial, before, after)
				}
				if original.Identity == spanNotMe && span.Identity != spanNotMe {
					t.Fatalf("trial %d changed a rejection: before=%+v after=%+v", trial, before, after)
				}
			}
		}
		if end != n {
			t.Fatalf("trial %d truncated at %d/%d", trial, end, n)
		}
	}
}
