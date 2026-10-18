package codec

import (
	"context"
	"io"
	"math"
)

// Blackman-windowed sinc with a low-pass cutoff below the destination Nyquist
// frequency prevents high-frequency input from aliasing into speech. Rational
// phases reuse kernels, and the rolling input buffer is bounded by a block plus
// the filter width. Existing 16 kHz recordings bypass the filter entirely.
func resample(ctx context.Context, source *monoReader, rate int) ([]float32, error) {
	result := make([]float32, 0, SampleRate)
	block := make([]float64, 4096)
	if rate == SampleRate {
		for {
			n, err := source.Read(block)
			for _, value := range block[:n] {
				result = append(result, float32(value))
			}
			if err == io.EOF {
				return result, nil
			}
			if err != nil {
				return nil, err
			}
		}
	}
	cutoff := math.Min(1, float64(SampleRate)/float64(rate)) * .94
	radius := int(math.Ceil(24 / cutoff))
	kernels := make(map[int][]float64)
	buffer := make([]float64, 0, 4096+2*radius)
	offset, total := int64(0), int64(0)
	eof := false
	for output := int64(0); ; output++ {
		if output%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		numerator := output * int64(rate)
		center, phase := numerator/SampleRate, int(numerator%SampleRate)
		for !eof && offset+int64(len(buffer)) <= center+int64(radius) {
			n, err := source.Read(block)
			buffer = append(buffer, block[:n]...)
			total += int64(n)
			if err == io.EOF {
				eof = true
			} else if err != nil {
				return nil, err
			}
		}
		if eof && numerator >= total*SampleRate {
			return result, nil
		}
		weights, ok := kernels[phase]
		if !ok {
			weights = make([]float64, 2*radius)
			sum := 0.0
			for i := range weights {
				x := float64(i-radius+1) - float64(phase)/SampleRate
				window := .42 + .5*math.Cos(math.Pi*x/float64(radius)) + .08*math.Cos(2*math.Pi*x/float64(radius))
				sinc := cutoff
				if x != 0 {
					sinc = math.Sin(math.Pi*cutoff*x) / (math.Pi * x)
				}
				weights[i] = window * sinc
				sum += weights[i]
			}
			for i := range weights {
				weights[i] /= sum
			}
			kernels[phase] = weights
		}
		value := 0.0
		for i, weight := range weights {
			index := min(max(center+int64(i-radius+1), 0), total-1)
			value += weight * buffer[index-offset]
		}
		result = append(result, float32(value))
		if drop := center - int64(radius) - offset; drop >= 4096 {
			copy(buffer, buffer[drop:])
			buffer = buffer[:int64(len(buffer))-drop]
			offset += drop
		}
	}
}
