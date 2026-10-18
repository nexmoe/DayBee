package voiceprint

import (
	"math"
	"math/rand/v2"
	"testing"
)

// Independent acceptance properties: every input sample remains available to
// ASR, and a hard echo exclusion never inherits a neighboring speaker badge.
func TestIdentitySpansCoverageAndEchoContract(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261012, 81))
	for trial := 0; trial < 400; trial++ {
		n := 12800 + rng.IntN(300000)
		windows := []WindowScore{}
		for start := 0; start < n; start += 8000 {
			windows = append(windows, WindowScore{Start: start, End: min(n, start+24000)})
		}
		scores, echo := make([]float32, len(windows)), make([]bool, len(windows))
		for i := range scores {
			scores[i] = []float32{-.8, .3, .9}[rng.IntN(3)]
			echo[i] = rng.IntN(7) == 0
		}
		spans := IdentitySpans(windows, scores, echo, n, 16000, .2, .6)
		at := 0
		for _, s := range spans {
			if s.Start != at || s.End <= s.Start || s.End > n {
				t.Fatalf("trial %d: discontinuity at %d: %+v", trial, at, spans)
			}
			if s.Identity != "me" && s.Identity != "not_me" && s.Identity != "uncertain" {
				t.Fatalf("trial %d: invalid identity: %+v", trial, s)
			}
			if math.IsNaN(float64(s.Score)) || math.IsInf(float64(s.Score), 0) {
				t.Fatalf("trial %d: invalid score: %+v", trial, s)
			}
			if s.Identity == "me" {
				for i, excluded := range echo {
					end := n
					if i+1 < len(windows) {
						end = windows[i+1].Start
					}
					if excluded && s.Start < end && s.End > windows[i].Start {
						t.Fatalf("trial %d: echo window %d inherited me: %+v", trial, i, spans)
					}
				}
			}
			at = s.End
		}
		if at != n {
			t.Fatalf("trial %d: lost tail %d/%d", trial, at, n)
		}
	}
}
