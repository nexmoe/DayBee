package audio

import (
	"math"
	"math/rand/v2"
	"testing"
)

// An independent DFT checks every bin, including ring-buffer wraparound,
// repeated smoothing and silence after sound. Chromium's fixture covers tones.
func TestSpectrumMatchesDirectTransform(t *testing.T) {
	rng := rand.New(rand.NewPCG(12, 34))
	s := NewSpectrum()
	var smoothed [128]float64
	for frame := 0; frame < 12; frame++ {
		samples := make([]float32, 173)
		for i := range samples {
			if frame < 6 {
				samples[i] = float32((rng.Float64() - .5) * .01)
			}
		}
		s.Push(samples)
		got := s.Frame()
		for k := range smoothed {
			var re, im float64
			for n := 0; n < 256; n++ {
				phase := 2 * math.Pi * float64(n) / 256
				window := .42 - .5*math.Cos(phase) + .08*math.Cos(2*phase)
				x := s.samples[(s.position+n)%256] * window
				re += x * math.Cos(phase*float64(k))
				im -= x * math.Sin(phase*float64(k))
			}
			smoothed[k] = .8*smoothed[k] + .2*math.Hypot(re, im)/256
			db := 20 * math.Log10(math.Max(smoothed[k], 1e-20))
			want := int(math.Floor(255 * math.Max(0, math.Min(1, (db+100)/70))))
			if math.Abs(float64(got[k]-want)) > 1 {
				t.Fatalf("frame %d bin %d: got %d want %d", frame, k, got[k], want)
			}
		}
	}
}

func BenchmarkSpectrumFrame(b *testing.B) {
	s := NewSpectrum()
	samples := make([]float32, 512)
	for i := range samples {
		samples[i] = float32(.01 * math.Sin(2*math.Pi*16*float64(i)/256))
	}
	s.Push(samples)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s.Frame()
	}
}
