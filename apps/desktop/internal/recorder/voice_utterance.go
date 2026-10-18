package recorder

import (
	"math"

	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

// utteranceVerdict reads confidence spans for one VAD utterance.
// Those spans are not speaker turns. The whole buffer is decoded once.
// me is stored only when the spans cover the utterance without gaps or overlap,
// every span is me, and the profile may label it. A non-finite score makes the
// utterance unknown. A mix of me and anything else is partial and cannot seed.
func utteranceVerdict(spans []voiceprint.IdentitySpan, n int, allow bool) (identity, match string, score float64, seed bool) {
	if !spansCover(spans, n) {
		return "", "", 0, false
	}
	me, other, anchored := 0, 0, 0
	weight := 0
	sum := 0.0
	for _, span := range spans {
		width := span.End - span.Start
		weight += width
		value := float64(span.Score)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", "", 0, false
		}
		sum += value * float64(width)
		if span.Identity == "me" {
			me++
			if span.Anchor {
				anchored++
			}
			continue
		}
		other++
	}
	if weight <= 0 {
		return "", "", 0, false
	}
	score = sum / float64(weight)
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return "", "", 0, false
	}
	if !allow || me == 0 {
		return "", "", score, false
	}
	if other > 0 {
		return "", "partial", score, false
	}
	return "me", "", score, anchored == me
}
