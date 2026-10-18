package voiceprint

import (
	"math/rand/v2"
	"testing"
)

const (
	evalEnrollSamples = 24 * voiceSampleRate
	evalGapSamples    = voiceSampleRate / 5
	evalKindTarget    = 0
	evalKindOther     = 1
	evalKindEcho      = 2
)

type evalUtt struct {
	index   int
	samples []float32
}

type evalTrial struct {
	enrolled int
	bucket   int
	kind     int
	source   int
	start    int
	length   int
}

type evalBucket struct {
	name                string
	lo, hi              int
	target, other, echo int
}

func evalBuckets() []evalBucket {
	return []evalBucket{
		{name: "1-2s", lo: voiceSampleRate, hi: 2*voiceSampleRate - 1, target: 16, other: 40, echo: 40},
		{name: "2-4s", lo: 2 * voiceSampleRate, hi: 4*voiceSampleRate - 1, target: 16, other: 40, echo: 40},
		{name: ">4s", lo: 4 * voiceSampleRate, hi: 8 * voiceSampleRate, target: 12, other: 40, echo: 40},
	}
}

func evalConcatPCM(parts [][]float32, gap int) []float32 {
	n := 0
	for i, part := range parts {
		n += len(part)
		if gap > 0 && i+1 < len(parts) {
			n += gap
		}
	}
	out := make([]float32, 0, n)
	for i, part := range parts {
		if i > 0 && gap > 0 {
			out = append(out, make([]float32, gap)...)
		}
		out = append(out, part...)
	}
	return out
}

// evalSplitEnroll keeps up to 24 s from the earliest utterances and holds the rest out.
// Target trials use only the held-out audio, so enrollment samples do not leak into them.
func evalSplitEnroll(utts []evalUtt) (enroll, test []float32, from int, ok bool) {
	for _, minTest := range []int{12 * voiceSampleRate, 4*voiceSampleRate + voiceSampleRate/2} {
		if enroll, test, from, ok = evalSplitAt(utts, minTest); ok {
			return enroll, test, from, true
		}
	}
	return nil, nil, 0, false
}

func evalSplitAt(utts []evalUtt, minTest int) (enroll, test []float32, from int, ok bool) {
	if len(utts) < 2 {
		return nil, nil, 0, false
	}
	bestK, bestUsed := -1, -1
	for k := 1; k < len(utts); k++ {
		enrollN, testN := 0, 0
		for i := 0; i < k; i++ {
			enrollN += len(utts[i].samples)
		}
		for i := k; i < len(utts); i++ {
			testN += len(utts[i].samples)
		}
		if enrollN < windowSamples || testN < minTest {
			continue
		}
		used := enrollN
		if used > evalEnrollSamples {
			used = evalEnrollSamples
		}
		if used > bestUsed {
			bestUsed = used
			bestK = k
		}
	}
	if bestK < 0 {
		return nil, nil, 0, false
	}
	parts := make([][]float32, bestK)
	for i := 0; i < bestK; i++ {
		parts[i] = utts[i].samples
	}
	enroll = evalConcatPCM(parts, 0)
	if len(enroll) > evalEnrollSamples {
		trimmed := make([]float32, evalEnrollSamples)
		copy(trimmed, enroll)
		enroll = trimmed
	}
	held := make([][]float32, len(utts)-bestK)
	for i := range held {
		held[i] = utts[bestK+i].samples
	}
	return enroll, evalConcatPCM(held, evalGapSamples), bestK, true
}

func evalOtherIndex(rng *rand.Rand, self, n int) int {
	if n < 2 {
		return -1
	}
	index := rng.IntN(n - 1)
	if index >= self {
		index++
	}
	return index
}

// evalAlignedCrop draws a hop-aligned start so full 1.5 s windows hit the precomputed grid.
// length is in [lo, hi], inclusive.
func evalAlignedCrop(rng *rand.Rand, n, lo, hi int) (start, length int, ok bool) {
	if hi > n {
		hi = n
	}
	if n < lo || hi < lo {
		return 0, 0, false
	}
	length = lo + rng.IntN(hi-lo+1)
	maxStart := n - length
	start = rng.IntN(maxStart/hopSamples+1) * hopSamples
	if start+length > n {
		return 0, 0, false
	}
	return start, length, true
}

func evalBuildTrials(lengths func(source, kind int) int, nSpeakers int) []evalTrial {
	rng := rand.New(rand.NewPCG(20261011, 1))
	buckets := evalBuckets()
	var out []evalTrial
	for enrolled := 0; enrolled < nSpeakers; enrolled++ {
		for b, bucket := range buckets {
			quotas := []struct{ kind, n int }{
				{evalKindTarget, bucket.target},
				{evalKindOther, bucket.other},
				{evalKindEcho, bucket.echo},
			}
			for _, quota := range quotas {
				filled := 0
				for tries := 0; filled < quota.n && tries < quota.n*8; tries++ {
					source := enrolled
					if quota.kind != evalKindTarget {
						source = evalOtherIndex(rng, enrolled, nSpeakers)
					}
					if source < 0 {
						break
					}
					lo, hi := bucket.lo, bucket.hi
					if b == 0 {
						if filled%2 == 0 {
							lo, hi = voiceSampleRate, windowSamples-1
						} else {
							lo, hi = windowSamples, 2*voiceSampleRate-1
						}
					}
					start, length, ok := evalAlignedCrop(rng, lengths(source, quota.kind), lo, hi)
					if !ok {
						continue
					}
					out = append(out, evalTrial{
						enrolled: enrolled, bucket: b, kind: quota.kind,
						source: source, start: start, length: length,
					})
					filled++
				}
			}
		}
	}
	return out
}

func TestVoiceprintEvalSplit(t *testing.T) {
	utts := make([]evalUtt, 10)
	for i := range utts {
		utts[i].index = i + 1
		utts[i].samples = make([]float32, 5*voiceSampleRate)
		for j := range utts[i].samples {
			utts[i].samples[j] = float32(i + 1)
		}
	}
	enroll, test, from, ok := evalSplitEnroll(utts)
	if !ok || from != 5 || len(enroll) != evalEnrollSamples {
		t.Fatalf("ok %v from %d enroll %d", ok, from, len(enroll))
	}
	if enroll[0] != 1 || enroll[len(enroll)-1] != 5 {
		t.Fatal("enrollment used the wrong utterances")
	}
	want := 5*5*voiceSampleRate + 4*evalGapSamples
	if len(test) != want || test[0] != 6 {
		t.Fatalf("test len %d first %v", len(test), test[0])
	}
}

func TestVoiceprintEvalTrials(t *testing.T) {
	lengths := func(int, int) int { return 30 * voiceSampleRate }
	trials := evalBuildTrials(lengths, 12)
	if len(trials) != 12*284 {
		t.Fatalf("trials %d", len(trials))
	}
	again := evalBuildTrials(lengths, 12)
	for i, trial := range trials {
		if trial.kind != evalKindTarget && trial.source == trial.enrolled {
			t.Fatal("non-target trial used the enrolled speaker")
		}
		if again[i] != trial {
			t.Fatal("trial draw changed across runs")
		}
	}
}
