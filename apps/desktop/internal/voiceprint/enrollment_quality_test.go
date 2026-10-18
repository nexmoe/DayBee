package voiceprint

import (
	"math"
	"testing"
)

func TestNoisePowerUsesLowQuantileAndFloor(t *testing.T) {
	if NoisePower(nil) != EnrollmentNoiseFloor || NoisePower([]float64{0, 0}) != EnrollmentNoiseFloor {
		t.Fatal("silent noise had no floor")
	}
	powers := []float64{0.5, 0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09}
	got := NoisePower(powers)
	if math.Abs(got-0.02) > 1e-12 {
		t.Fatalf("low quantile = %g, a loud frame became the noise estimate", got)
	}
}

func TestSpeechSNRIgnoresZeroNoise(t *testing.T) {
	quiet := SpeechSNR([]float32{0.1, -0.1}, 0.01)
	if math.Abs(quiet) > 1e-5 {
		t.Fatalf("equal speech and noise SNR = %g", quiet)
	}
	silentRoom := SpeechSNR([]float32{0.1, -0.1}, 0)
	if silentRoom < EnrollmentMinSNRdB || math.IsInf(silentRoom, 0) {
		t.Fatalf("zero noise SNR = %g", silentRoom)
	}
	if SpeechSNR(nil, EnrollmentNoiseFloor) >= EnrollmentMinSNRdB {
		t.Fatal("empty speech passed the SNR gate")
	}
}

func TestNonFiniteAudioCannotPassSNR(t *testing.T) {
	nan32 := float32(math.NaN())
	samples := [][]float32{
		{0.2, nan32},
		{float32(math.Inf(1))},
		{float32(math.Inf(-1)), 0.2},
	}
	for _, speech := range samples {
		power := MeanSquare(speech)
		if finite64(power) {
			t.Fatalf("non-finite PCM power = %g", power)
		}
		snr := SpeechSNR(speech, 0.001)
		if math.IsNaN(snr) || !math.IsInf(snr, -1) || !(snr < EnrollmentMinSNRdB) || AssessEnrollment(speech, 0.001) != QualityInvalid {
			t.Fatalf("PCM SNR = %g failure = %d", snr, AssessEnrollment(speech, 0.001))
		}
	}
	for _, noise := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.01} {
		snr := SpeechSNR([]float32{0.2, -0.2}, noise)
		if math.IsNaN(snr) || snr >= EnrollmentMinSNRdB || AssessEnrollment([]float32{0.2, -0.2}, noise) != QualityInvalid {
			t.Fatalf("noise %g produced SNR %g", noise, snr)
		}
	}
	for _, powers := range [][]float64{{0.01, math.NaN()}, {math.Inf(1)}, {0.02, math.Inf(-1)}, {-0.1, 0.01}} {
		noise := NoisePower(powers)
		speech := []float32{0.2, -0.2}
		snr := SpeechSNR(speech, noise)
		if math.IsNaN(noise) || math.IsNaN(snr) || snr >= EnrollmentMinSNRdB || AssessEnrollment(speech, noise) == QualityOK {
			t.Fatalf("powers %v noise %g SNR %g", powers, noise, snr)
		}
	}
}

func TestInconsistentEnrollmentRequiresAbsoluteGap(t *testing.T) {
	same := []float32{1, 0}
	close := []float32{0.95, float32(math.Sqrt(1 - 0.95*0.95))}
	if InconsistentEnrollment([][][]float32{{same}, {same}, {close}}) {
		t.Fatal("a 0.025 median gap was rejected")
	}
	unlike := []float32{0, 1}
	if !InconsistentEnrollment([][][]float32{{same}, {same}, {unlike}}) {
		t.Fatal("one unlike group was accepted")
	}
	// Pairwise cosines are [1, 0, 0]. Their median is 0, so a pairwise median
	// rule would not see an outlier. Group scores still do.
	pairs := []float32{Cosine(same, same), Cosine(same, unlike), Cosine(same, unlike)}
	if medianScore(pairs) != 0 {
		t.Fatalf("pairwise median = %g", medianScore(pairs))
	}
}

func TestInconsistentEnrollmentRejectsSeparatedGroup(t *testing.T) {
	same := []float32{1, 0}
	apart := []float32{0.4, float32(math.Sqrt(1 - 0.16))}
	if Cosine(same, apart) < 0.39 || Cosine(same, apart) > 0.41 {
		t.Fatalf("fixture cosine = %g", Cosine(same, apart))
	}
	if !InconsistentEnrollment([][][]float32{{same}, {append([]float32(nil), same...)}, {apart}}) {
		t.Fatal("a group 0.4 away from the other two was accepted")
	}
	if !InconsistentEnrollment([][][]float32{{same}, {}}) {
		t.Fatal("an incomplete enrollment was accepted")
	}
}
