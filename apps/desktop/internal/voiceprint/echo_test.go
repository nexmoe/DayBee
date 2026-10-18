package voiceprint

import (
	"math"
	"testing"
)

func TestEnvelopeCorrelationDelayedCopyAndNoise(t *testing.T) {
	const rate = 16000
	const delay = 80 * rate / 1000
	ref := pulsed(2*rate, 1)
	mic := delayFilter(ref, delay)
	frame := rate * echoFrameMS / 1000
	rho, lag := envelopeXCorr(frameLogEnergy(mic, frame), frameLogEnergy(ref, frame), echoMaxLagMS/echoFrameMS)
	if rho < 0.8 || lag < delay/frame-1 || lag > delay/frame+1 {
		t.Fatalf("delayed copy rho = %v lag = %d", rho, lag)
	}
	other := pulsed(2*rate, 99)
	noise, _ := envelopeXCorr(frameLogEnergy(mic, frame), frameLogEnergy(other, frame), echoMaxLagMS/echoFrameMS)
	if noise > 0.45 {
		t.Fatalf("independent envelopes rho = %v", noise)
	}
}

func TestEchoByScore(t *testing.T) {
	threshold := Threshold(ModelName)
	if !echoByScore(0.5, true, 0, 0.5, threshold) {
		t.Fatal("system score within the margin was kept")
	}
	if echoByScore(0.4, true, 0, 0.5, threshold) {
		t.Fatal("system score below the margin was echo")
	}
	if !echoByScore(0, false, echoRhoMin, threshold, threshold) {
		t.Fatal("correlated low score was kept")
	}
	if echoByScore(0, false, echoRhoMin, threshold+echoSlack, threshold) || echoByScore(0, false, echoRhoMin-0.01, 0, threshold) {
		t.Fatal("confident or weakly correlated score was echo")
	}
}

func TestEchoWindowsRejectsSilence(t *testing.T) {
	if EchoWindows == nil {
		t.Fatal("EchoWindows was not registered")
	}
	const rate = 16000
	mic := pulsed(2*rate, 3)
	wins := []WindowScore{{Start: 0, End: len(mic), Score: 0.2, Embedding: []float32{1, 0}}}
	silent := EchoWindows(&Manager{}, mic, &Reference{Samples: make([]float32, len(mic))}, wins, rate)
	quietSamples := make([]float32, len(mic))
	for i := range quietSamples {
		quietSamples[i] = 1e-6
	}
	quiet := EchoWindows(&Manager{}, mic, &Reference{Samples: quietSamples}, wins, rate)
	if len(silent) != 1 || silent[0] || len(quiet) != 1 || quiet[0] {
		t.Fatalf("silence = %v quiet = %v", silent, quiet)
	}
	delayed := delayFilter(mic, 80*rate/1000)
	echo := EchoWindows(&Manager{}, delayed, &Reference{Samples: mic}, wins, rate)
	wins[0].Score = 0.95
	kept := EchoWindows(&Manager{}, delayed, &Reference{Samples: mic}, wins, rate)
	if len(echo) != 1 || !echo[0] || len(kept) != 1 || kept[0] {
		t.Fatalf("delayed low score = %v high score = %v", echo, kept)
	}
	if math.IsNaN(float64(echoRhoMin)) {
		t.Fatal("rho threshold is not finite")
	}
}

func TestEchoReusesIdenticalMicEmbedding(t *testing.T) {
	const rate = 16000
	samples := make([]float32, rate)
	for i := range samples {
		samples[i] = 0.2
	}
	wins := []WindowScore{{Start: 0, End: len(samples), Score: 0.2, Embedding: []float32{1, 0}}}
	flags := EchoWindows(&Manager{}, samples, &Reference{Samples: append([]float32(nil), samples...)}, wins, rate)
	if len(flags) != 1 || !flags[0] {
		t.Fatalf("identical system audio was embedded again: %v", flags)
	}
}

func pulsed(n int, seed uint32) []float32 {
	next := lcg(seed)
	out := make([]float32, n)
	const block = 320
	for i := 0; i < n; {
		amp := float32(0.25 + 0.75*(float64(next())+1)/2)
		end := i + block
		if end > n {
			end = n
		}
		for ; i < end; i++ {
			out[i] = next() * amp
		}
	}
	return out
}

func lcg(seed uint32) func() float32 {
	return func() float32 {
		seed = seed*1664525 + 1013904223
		return float32(int32(seed)) / 2147483648
	}
}

func delayFilter(src []float32, delay int) []float32 {
	out := make([]float32, len(src))
	var a, b, c float32
	for i, sample := range src {
		filtered := 0.5*sample + 0.3*a + 0.15*b + 0.05*c
		c, b, a = b, a, sample
		if j := i + delay; j < len(out) {
			out[j] = filtered
		}
	}
	return out
}
