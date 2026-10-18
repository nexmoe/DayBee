package voiceprint

import (
	"math"
	"sort"
)

const (
	asNormTopK       = 100
	asNormSigmaFloor = 1e-3
	asNormSigmaCeil  = 1
	asNormMin        = -20
	asNormMax        = 20
)

// asNorm is the bounded symmetric score ½[(s-μe)/σe + (s-μt)/σt].
// Sigma is floored and capped. The result is finite and inside [asNormMin, asNormMax].
func asNorm(raw, enrollMean, enrollStd, testMean, testStd float64) float32 {
	z := 0.5 * ((raw-enrollMean)/enrollStd + (raw-testMean)/testStd)
	if z < asNormMin {
		z = asNormMin
	}
	if z > asNormMax {
		z = asNormMax
	}
	value := float32(z)
	if !finite32(value) {
		return asNormMin
	}
	return value
}

func normTop(scores []float64) (mean, std float64, ok bool) {
	clean := make([]float64, 0, len(scores))
	for _, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		clean = append(clean, score)
	}
	if len(clean) == 0 {
		return 0, 0, false
	}
	sort.Float64s(clean)
	k := asNormTopK
	if k > len(clean) {
		k = len(clean)
	}
	top := clean[len(clean)-k:]
	var sum float64
	for _, score := range top {
		sum += score
	}
	mean = sum / float64(len(top))
	var acc float64
	for _, score := range top {
		delta := score - mean
		acc += delta * delta
	}
	std = math.Sqrt(acc / float64(len(top)))
	if !(std >= asNormSigmaFloor) {
		std = asNormSigmaFloor
	}
	if std > asNormSigmaCeil {
		std = asNormSigmaCeil
	}
	if math.IsNaN(mean) || math.IsInf(mean, 0) || math.IsNaN(std) || math.IsInf(std, 0) {
		return 0, 0, false
	}
	return mean, std, true
}

func cohortCosines(vector []float32, cohort [][]float32) []float64 {
	scores := make([]float64, 0, len(cohort))
	for _, other := range cohort {
		score := Cosine(vector, other)
		if finite32(score) {
			scores = append(scores, float64(score))
		}
	}
	return scores
}

func cohortProfileScores(profile *Profile, cohort [][]float32) []float64 {
	scores := make([]float64, 0, len(cohort))
	for _, other := range cohort {
		score := ProfileScore(profile, other)
		if finite32(score) {
			scores = append(scores, float64(score))
		}
	}
	return scores
}

type sideStats struct{ mean, std float64 }

func sideNorms(vectors, cohort [][]float32) ([]sideStats, bool) {
	out := make([]sideStats, len(vectors))
	for i, vector := range vectors {
		mean, std, ok := normTop(cohortCosines(vector, cohort))
		if !ok {
			return nil, false
		}
		out[i] = sideStats{mean, std}
	}
	return out, true
}
