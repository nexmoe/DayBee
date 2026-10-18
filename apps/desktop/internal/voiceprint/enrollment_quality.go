package voiceprint

import (
	"math"
	"sort"
)

const (
	// EnrollmentMinSNRdB is the minimum net-speech SNR. Noise is a low quantile
	// of VAD-negative power, never speech energy.
	EnrollmentMinSNRdB = 15.0
	// EnrollmentNoiseFloor keeps a silent room from dividing by zero.
	EnrollmentNoiseFloor = 1e-12
	// EnrollmentOutlierGap is the smallest median-to-group gap that can reject a take.
	EnrollmentOutlierGap    = 0.10
	enrollmentNoiseQuantile = 0.20
)

type QualityFailure int

const (
	QualityOK QualityFailure = iota
	QualityInvalid
	QualityLowSNR
)

func finite64(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// FinitePCM reports whether every sample is a finite number. Empty audio is not usable.
func FinitePCM(samples []float32) bool {
	if len(samples) == 0 {
		return false
	}
	for _, sample := range samples {
		if !finite64(float64(sample)) {
			return false
		}
	}
	return true
}

// MeanSquare is the average sample power. Empty audio is zero.
// Non-finite PCM returns NaN so callers cannot treat it as silence.
func MeanSquare(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		value := float64(sample)
		if !finite64(value) {
			return math.NaN()
		}
		sum += value * value
	}
	power := sum / float64(len(samples))
	if !finite64(power) {
		return math.NaN()
	}
	return power
}

// NoisePower is the low-energy quantile of VAD-negative frame powers.
// Values below the floor, including all-zero noise, use the floor.
// A non-finite or negative frame returns +Inf, which cannot clear the SNR gate.
func NoisePower(framePowers []float64) float64 {
	if len(framePowers) == 0 {
		return EnrollmentNoiseFloor
	}
	ordered := make([]float64, len(framePowers))
	for i, power := range framePowers {
		if !finite64(power) || power < 0 {
			return math.Inf(1)
		}
		ordered[i] = power
	}
	sort.Float64s(ordered)
	index := int(math.Floor(enrollmentNoiseQuantile * float64(len(ordered)-1)))
	if index < 0 {
		index = 0
	}
	power := ordered[index]
	if !finite64(power) {
		return math.Inf(1)
	}
	if power < EnrollmentNoiseFloor {
		return EnrollmentNoiseFloor
	}
	return power
}

// SpeechSNR is 10 log10(speechPower / noisePower) for net speech.
// Invalid PCM or noise returns -Inf, never NaN or +Inf, so an SNR < 15 check rejects it.
func SpeechSNR(speech []float32, noisePower float64) float64 {
	if !FinitePCM(speech) || !finite64(noisePower) || noisePower < 0 {
		return math.Inf(-1)
	}
	if noisePower < EnrollmentNoiseFloor {
		noisePower = EnrollmentNoiseFloor
	}
	speechPower := MeanSquare(speech)
	if !finite64(speechPower) || speechPower <= 0 {
		return math.Inf(-1)
	}
	snr := 10 * math.Log10(speechPower/noisePower)
	if !finite64(snr) {
		return math.Inf(-1)
	}
	return snr
}

// AssessEnrollment rejects non-finite audio before the numeric threshold is considered.
func AssessEnrollment(speech []float32, noisePower float64) QualityFailure {
	if !FinitePCM(speech) || !finite64(noisePower) || noisePower < 0 {
		return QualityInvalid
	}
	snr := SpeechSNR(speech, noisePower)
	if !finite64(snr) {
		return QualityInvalid
	}
	if snr < EnrollmentMinSNRdB {
		return QualityLowSNR
	}
	return QualityOK
}

// InconsistentEnrollment is true when one group's mean cosine to the other
// groups falls more than max(2σ, 0.10) below the median of those group scores.
// σ is the population standard deviation of the group scores. Pairwise medians
// are not used, because one bad group would hide in two low pairs.
func InconsistentEnrollment(groups [][][]float32) bool {
	if len(groups) < 3 {
		return true
	}
	centroids := make([][]float32, len(groups))
	for i, group := range groups {
		centroids[i] = groupCentroid(group)
		if centroids[i] == nil {
			return true
		}
	}
	return enrollmentOutlier(groupAgreement(centroids))
}

func groupCentroid(group [][]float32) []float32 {
	if len(group) == 0 || len(group[0]) == 0 {
		return nil
	}
	dim := len(group[0])
	sum := make([]float64, dim)
	for _, vector := range group {
		if len(vector) != dim {
			return nil
		}
		for i, value := range vector {
			sum[i] += float64(value)
		}
	}
	centroid := make([]float32, dim)
	scale := float64(len(group))
	for i := range centroid {
		centroid[i] = float32(sum[i] / scale)
	}
	return centroid
}

func groupAgreement(centroids [][]float32) []float32 {
	scores := make([]float32, len(centroids))
	for i := range centroids {
		var sum float64
		var others int
		for j := range centroids {
			if i == j {
				continue
			}
			sum += float64(Cosine(centroids[i], centroids[j]))
			others++
		}
		if others == 0 {
			continue
		}
		scores[i] = float32(sum / float64(others))
	}
	return scores
}

func enrollmentOutlier(scores []float32) bool {
	if len(scores) < 3 {
		return true
	}
	middle := medianScore(scores)
	limit := math.Max(2*populationSigma(scores), EnrollmentOutlierGap)
	for _, score := range scores {
		if float64(middle)-float64(score) > limit {
			return true
		}
	}
	return false
}

func medianScore(scores []float32) float32 {
	ordered := append([]float32(nil), scores...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	mid := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[mid]
	}
	return (ordered[mid-1] + ordered[mid]) / 2
}

func populationSigma(scores []float32) float64 {
	var sum float64
	for _, score := range scores {
		sum += float64(score)
	}
	mean := sum / float64(len(scores))
	var square float64
	for _, score := range scores {
		delta := float64(score) - mean
		square += delta * delta
	}
	return math.Sqrt(square / float64(len(scores)))
}
