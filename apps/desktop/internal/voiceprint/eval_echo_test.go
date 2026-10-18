package voiceprint

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"
)

func evalRNG(id string, index int) *rand.Rand {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(id))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(index))
	_, _ = hash.Write(buf[:])
	return rand.New(rand.NewPCG(hash.Sum64(), 0xE3A11))
}

// evalEchoResidue is an AEC leftover of playback from another speaker:
// a 200 ms exponential-decay noise room impulse (RT60 ≈ 200 ms),
// a 40–120 ms delay, −20 dB, then pink noise at 15 dB SNR versus that residue.
func evalEchoResidue(rng *rand.Rand, samples []float32) []float32 {
	wet := evalConvolve(samples, evalRoomIR(rng))
	for i := range wet {
		wet[i] *= 0.1
	}
	delay := (40 + rng.IntN(81)) * voiceSampleRate / 1000
	out := make([]float32, delay+len(wet))
	copy(out[delay:], wet)
	noise := evalPinkNoise(rng, len(out))
	var signalPower, noisePower float64
	for _, sample := range wet {
		signalPower += float64(sample) * float64(sample)
	}
	for _, sample := range noise {
		noisePower += float64(sample) * float64(sample)
	}
	signalPower /= float64(len(wet))
	noisePower /= float64(len(noise))
	var scale float64
	if signalPower > 0 && noisePower > 0 {
		scale = math.Sqrt(signalPower / (math.Pow(10, 1.5) * noisePower))
	}
	for i := range out {
		out[i] += float32(scale * float64(noise[i]))
	}
	return out
}

func evalRoomIR(rng *rand.Rand) []float32 {
	n := voiceSampleRate / 5
	ir := make([]float32, n)
	ir[0] = 1
	decay := math.Log(1000) / 0.2 // amplitude is down 60 dB at 200 ms
	var energy float64
	for i := 1; i < n; i++ {
		t := float64(i) / float64(voiceSampleRate)
		sample := rng.NormFloat64() * math.Exp(-t*decay)
		ir[i] = float32(sample)
		energy += sample * sample
	}
	if energy > 0 {
		scale := float32(1 / math.Sqrt(energy))
		for i := 1; i < n; i++ {
			ir[i] *= scale
		}
	}
	return ir
}

// evalPinkNoise is a Kellet filter. The caller rescales it to the requested SNR.
func evalPinkNoise(rng *rand.Rand, n int) []float32 {
	var b0, b1, b2, b3, b4, b5, b6 float64
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		white := rng.NormFloat64()
		b0 = 0.99886*b0 + white*0.0555179
		b1 = 0.99332*b1 + white*0.0750759
		b2 = 0.96900*b2 + white*0.1538520
		b3 = 0.86650*b3 + white*0.3104856
		b4 = 0.55000*b4 + white*0.5329522
		b5 = -0.7616*b5 - white*0.0168980
		out[i] = float32(b0 + b1 + b2 + b3 + b4 + b5 + b6 + white*0.5362)
		b6 = white * 0.115926
	}
	return out
}

func evalConvolve(x, h []float32) []float32 {
	if len(x) == 0 || len(h) == 0 {
		return nil
	}
	n := len(x) + len(h) - 1
	nfft := 1
	for nfft < n {
		nfft <<= 1
	}
	a := make([]complex128, nfft)
	b := make([]complex128, nfft)
	for i, value := range x {
		a[i] = complex(float64(value), 0)
	}
	for i, value := range h {
		b[i] = complex(float64(value), 0)
	}
	evalFFT(a, false)
	evalFFT(b, false)
	for i := range a {
		a[i] *= b[i]
	}
	evalFFT(a, true)
	out := make([]float32, n)
	scale := 1 / float64(nfft)
	for i := 0; i < n; i++ {
		out[i] = float32(real(a[i]) * scale)
	}
	return out
}

func evalFFT(a []complex128, invert bool) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := 2 * math.Pi / float64(length)
		if invert {
			ang = -ang
		}
		wlen := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += length {
			w := 1 + 0i
			half := length >> 1
			for j := 0; j < half; j++ {
				u := a[i+j]
				v := a[i+j+half] * w
				a[i+j] = u + v
				a[i+j+half] = u - v
				w *= wlen
			}
		}
	}
}

func TestVoiceprintEvalConvolve(t *testing.T) {
	got := evalConvolve([]float32{1, 2, 3, 4}, []float32{1, -1})
	want := []float32{1, 1, 1, 1, -4}
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-4 {
			t.Fatalf("sample %d = %f", i, got[i])
		}
	}
}

func TestVoiceprintEvalEchoLevel(t *testing.T) {
	src := make([]float32, voiceSampleRate)
	for i := range src {
		src[i] = 0.2
	}
	out := evalEchoResidue(rand.New(rand.NewPCG(9, 7)), src)
	if len(out) <= len(src) {
		t.Fatalf("echo length %d", len(out))
	}
	var srcPow, outPow float64
	for _, sample := range src {
		srcPow += float64(sample) * float64(sample)
	}
	for _, sample := range out {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			t.Fatal("non-finite echo sample")
		}
		outPow += float64(sample) * float64(sample)
	}
	ratio := math.Sqrt((outPow / float64(len(out))) / (srcPow / float64(len(src))))
	if ratio > 0.5 || ratio < 0.005 {
		t.Fatalf("residue rms ratio %f", ratio)
	}
}
