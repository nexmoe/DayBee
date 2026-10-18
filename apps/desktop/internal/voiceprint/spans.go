package voiceprint

import "math"

const (
	spanMe        = "me"
	spanNotMe     = "not_me"
	spanUncertain = "uncertain"
)

// IdentitySpan is one half-open sample range [Start, End).
// Identity is me, not_me, or uncertain.
// Score is the smoothed window score weighted by owned samples.
// Anchor is set only for me that also passes DecideProfile at the original
// high threshold. A me saved only by relaxation stays false.
type IdentitySpan struct {
	Start, End int
	Identity   string
	Score      float32
	Anchor     bool
}

// IdentitySpans covers [0, n) with contiguous, non-overlapping spans.
// Window i owns [start, nextStart); the first window starts at 0 and the last ends at n.
// scores pass through the 3-point median, then low/high hysteresis.
// A new identity is kept only when that run lasts at least one second.
// Echo is not_me even below one second, and it is not folded into me.
// Missing scores or invalid arguments become one uncertain span. Empty audio is nil.
func IdentitySpans(windows []WindowScore, scores []float32, echo []bool, n, rate int, low, high float32) []IdentitySpan {
	if n <= 0 {
		return nil
	}
	if !spanArgsOK(windows, scores, echo, n, rate, low, high) {
		return []IdentitySpan{{Start: 0, End: n, Identity: spanUncertain}}
	}
	bounds := spanBounds(windows, n)
	smoothed := smoothScores(scores)
	rejected := hardReject(scores, echo)
	labels := hysteresisLabels(smoothed, rejected, low, high)
	sustainLabels(labels, rejected, bounds, rate)
	return mergeSpans(labels, smoothed, bounds)
}

// hardReject copies echo and marks windows whose original score is at the floor.
// Callers' echo slices are left unchanged. Short floor runs stay not-me.
func hardReject(scores []float32, echo []bool) []bool {
	mask := make([]bool, len(scores))
	for i, score := range scores {
		mask[i] = score <= asNormMin || (echo != nil && i < len(echo) && echo[i])
	}
	return mask
}

func spanArgsOK(windows []WindowScore, scores []float32, echo []bool, n, rate int, low, high float32) bool {
	if rate <= 0 || len(windows) == 0 || len(scores) != len(windows) {
		return false
	}
	if echo != nil && len(echo) != len(windows) {
		return false
	}
	if !finite32(low) || !finite32(high) || high < low {
		return false
	}
	for _, score := range scores {
		if !finite32(score) {
			return false
		}
	}
	prev := 0
	for i := 1; i < len(windows); i++ {
		start := windows[i].Start
		if start < prev || start > n {
			return false
		}
		prev = start
	}
	return true
}

func spanBounds(windows []WindowScore, n int) []int {
	bounds := make([]int, len(windows)+1)
	for i := 1; i < len(windows); i++ {
		bounds[i] = windows[i].Start
	}
	bounds[len(windows)] = n
	return bounds
}

func hysteresisLabels(scores []float32, echo []bool, low, high float32) []string {
	labels := make([]string, len(scores))
	state := ""
	for i, score := range scores {
		if echo != nil && echo[i] {
			labels[i] = spanNotMe
			continue
		}
		state = nextSpanIdentity(state, score, low, high)
		labels[i] = state
	}
	return labels
}

// Stay in me until the score falls below low. Enter me only at or above high.
// Echo does not move this state; it is overlaid later as not_me.
func nextSpanIdentity(prev string, score, low, high float32) string {
	if prev == spanMe && score >= low {
		return spanMe
	}
	if score >= high {
		return spanMe
	}
	if score < low {
		return spanNotMe
	}
	return spanUncertain
}

// Runs shorter than one second keep the previous identity.
// Echo windows are skipped, so a short echo cannot become the identity that
// later speech is absorbed into, and it is never rewritten to me.
func sustainLabels(labels []string, echo []bool, bounds []int, rate int) {
	accepted := ""
	for i := 0; i < len(labels); {
		if echo != nil && echo[i] {
			i++
			continue
		}
		j := i + 1
		for j < len(labels) && (echo == nil || !echo[j]) && labels[j] == labels[i] {
			j++
		}
		if accepted == "" || labels[i] == accepted || bounds[j]-bounds[i] >= rate {
			accepted = labels[i]
		} else {
			for k := i; k < j; k++ {
				labels[k] = accepted
			}
		}
		i = j
	}
}

func mergeSpans(labels []string, scores []float32, bounds []int) []IdentitySpan {
	out := make([]IdentitySpan, 0, len(labels))
	for i := 0; i < len(labels); {
		j := i + 1
		for j < len(labels) && labels[j] == labels[i] {
			j++
		}
		if bounds[j] > bounds[i] {
			out = append(out, IdentitySpan{
				Start: bounds[i], End: bounds[j], Identity: labels[i],
				Score: weightedSpanScore(scores, bounds, i, j),
			})
		}
		i = j
	}
	return out
}

func weightedSpanScore(scores []float32, bounds []int, i, j int) float32 {
	var sum float64
	weight := 0
	for k := i; k < j; k++ {
		width := bounds[k+1] - bounds[k]
		if width <= 0 {
			continue
		}
		sum += float64(scores[k]) * float64(width)
		weight += width
	}
	if weight == 0 {
		return 0
	}
	return float32(sum / float64(weight))
}

func finite32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}
