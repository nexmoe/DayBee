package voiceprint

const (
	defaultThreshold float32 = 0.55
	meWindowFraction         = 0.7
)

// modelThresholds is the accept threshold keyed by profile model file name.
// Values are the FA≤1% points from docs/verification/voiceprint-eval.json (1–2 s chunks).
var modelThresholds = map[string]float32{
	ModelName: 0.537,
}

func Threshold(model string) float32 {
	if threshold, ok := modelThresholds[model]; ok {
		return threshold
	}
	return defaultThreshold
}

func median3(a, b, c float32) float32 {
	if a > b {
		a, b = b, a
	}
	if b > c {
		b, c = c, b
	}
	if a > b {
		b = a
	}
	return b
}

func smoothScores(scores []float32) []float32 {
	out := make([]float32, len(scores))
	for i := range scores {
		if scores[i] <= asNormMin {
			out[i] = scores[i]
			continue
		}
		prev, next := scores[i], scores[i]
		if i > 0 {
			prev = scores[i-1]
		}
		if i+1 < len(scores) {
			next = scores[i+1]
		}
		out[i] = median3(prev, scores[i], next)
	}
	return out
}

// Decide smooths window scores and labels the chunk.
// Echo windows count as not-me. The chunk is "me" only when the fraction of
// smoothed scores at or above the model threshold is at least 0.7 and the mean
// smoothed score is at least that threshold.
func Decide(scores []float32, echo []bool, model string) (identity string, mean float32) {
	if len(scores) == 0 {
		return "", 0
	}
	smoothed := smoothScores(scores)
	threshold := Threshold(model)
	var sum float64
	pass := 0
	for i, score := range smoothed {
		sum += float64(score)
		if i < len(echo) && echo[i] {
			continue
		}
		if score >= threshold {
			pass++
		}
	}
	mean = float32(sum / float64(len(smoothed)))
	if float64(pass)/float64(len(smoothed)) >= meWindowFraction && mean >= threshold {
		return "me", mean
	}
	return "", mean
}

// DecideProfile labels a chunk on the DecisionScores scale.
// Echo windows count as not-me. "me" still needs 70% of windows at or above high and a mean at or above high.
// Scores between low and high are uncertain. A single legacy threshold has no uncertain band.
func DecideProfile(profile *Profile, scores []float32, echo []bool) (string, float32) {
	if len(scores) == 0 {
		return "", 0
	}
	if profile != nil && profile.Version >= calibratedVersion && !bandOK(profile.Calibration) {
		return "uncertain", 0
	}
	low, high := DecisionThresholds(profile)
	if !finite32(low) || !finite32(high) || high < low {
		if profile != nil && profile.Version >= calibratedVersion {
			return "uncertain", 0
		}
		return "", 0
	}
	for _, score := range scores {
		if !finite32(score) {
			return "", 0
		}
	}
	smoothed := smoothScores(scores)
	var sum float64
	pass, rejected := 0, 0
	for i, score := range smoothed {
		sum += float64(score)
		if (i < len(echo) && echo[i]) || score < low {
			rejected++
			continue
		}
		if score >= high {
			pass++
		}
	}
	mean := float32(sum / float64(len(smoothed)))
	if !finite32(mean) {
		return "", 0
	}
	if float64(pass)/float64(len(smoothed)) >= meWindowFraction && mean >= high {
		return "me", mean
	}
	if mean < low || rejected == len(smoothed) || high == low {
		return "not_me", mean
	}
	return "uncertain", mean
}
