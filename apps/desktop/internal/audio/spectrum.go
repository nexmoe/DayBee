package audio

import "math"

const SampleRate = 16000
const WindowSize = 512

// Spectrum implements the 256-point Blackman-windowed analyser used by the
// existing canvas: linear magnitude smoothing, then -100..-30 dB byte mapping.
// It is owned by the processing worker, never the native capture callback.
type Spectrum struct {
	samples  [256]float64
	position int
	smoothed [128]float64
}

func NewSpectrum() *Spectrum { return &Spectrum{} }

var spectrumWindow, spectrumTwiddle, spectrumOrder = spectrumTables()

func spectrumTables() ([256]float64, [128]complex128, [256]uint8) {
	var window [256]float64
	var twiddle [128]complex128
	var order [256]uint8
	for n := range window {
		phase := 2 * math.Pi * float64(n) / 256
		window[n] = .42 - .5*math.Cos(phase) + .08*math.Cos(2*phase)
		for bit := range 8 {
			order[n] = order[n]<<1 | uint8(n>>bit&1)
		}
	}
	for k := range twiddle {
		phase := -2 * math.Pi * float64(k) / 256
		twiddle[k] = complex(math.Cos(phase), math.Sin(phase))
	}
	return window, twiddle, order
}

func (s *Spectrum) Push(samples []float32) {
	for _, sample := range samples {
		s.samples[s.position] = float64(sample)
		s.position = (s.position + 1) % len(s.samples)
	}
}

func (s *Spectrum) Frame() []int {
	// A radix-2 FFT replaces 128 independent 256-sample dot products. Keep the
	// same Blackman window, normalization, smoothing and byte mapping.
	var bins [256]complex128
	for n := range bins {
		bins[spectrumOrder[n]] = complex(s.samples[(s.position+n)%256]*spectrumWindow[n], 0)
	}
	for size := 2; size <= len(bins); size *= 2 {
		half, step := size/2, len(bins)/size
		for start := 0; start < len(bins); start += size {
			for j := 0; j < half; j++ {
				even, odd := bins[start+j], bins[start+j+half]*spectrumTwiddle[j*step]
				bins[start+j], bins[start+j+half] = even+odd, even-odd
			}
		}
	}
	result := make([]int, 128)
	for k := range result {
		magnitude := math.Hypot(real(bins[k]), imag(bins[k])) / 256
		s.smoothed[k] = .8*s.smoothed[k] + .2*magnitude
		db := 20 * math.Log10(math.Max(s.smoothed[k], 1e-20))
		result[k] = int(math.Floor(255 * math.Max(0, math.Min(1, (db+100)/70))))
	}
	return result
}

func RMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func Decibels(rms float64) float64 { return math.Max(-80, 20*math.Log10(math.Max(rms, 1e-12))) }

func WaveformBins(samples []float32) []float64 {
	bins := make([]float64, 48)
	if len(samples) == 0 {
		return bins
	}
	bucket := max(1, len(samples)/len(bins))
	for i := range bins {
		start, end := min(len(samples), i*bucket), min(len(samples), (i+1)*bucket)
		for _, value := range samples[start:end] {
			bins[i] = max(bins[i], math.Abs(float64(value)))
		}
	}
	return bins
}
