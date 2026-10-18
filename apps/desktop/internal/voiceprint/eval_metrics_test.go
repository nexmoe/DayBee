package voiceprint

import (
	"math"
	"sort"
	"testing"
)

// evalChunkScore is the mean of 3-point median-smoothed window scores.
// That is the mean Decide already computes before the 0.7 window vote.
func evalChunkScore(scores []float32) float64 {
	if len(scores) == 0 {
		return math.NaN()
	}
	smoothed := smoothScores(scores)
	var sum float64
	for _, score := range smoothed {
		sum += float64(score)
	}
	return sum / float64(len(smoothed))
}

// evalEER interpolates the point where the false-accept rate equals the false-reject rate.
// A score at or above the threshold is an accept.
func evalEER(target, nontarget []float64) float64 {
	if len(target) == 0 || len(nontarget) == 0 {
		return math.NaN()
	}
	type scored struct {
		value    float64
		positive bool
	}
	all := make([]scored, 0, len(target)+len(nontarget))
	for _, value := range target {
		all = append(all, scored{value, true})
	}
	for _, value := range nontarget {
		all = append(all, scored{value, false})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].value > all[j].value })
	positives := float64(len(target))
	negatives := float64(len(nontarget))
	var truePos, falsePos float64
	prevFPR, prevFNR := 0.0, 1.0
	for i := 0; i < len(all); {
		value := all[i].value
		for i < len(all) && all[i].value == value {
			if all[i].positive {
				truePos++
			} else {
				falsePos++
			}
			i++
		}
		fpr := falsePos / negatives
		fnr := (positives - truePos) / positives
		d0 := prevFPR - prevFNR
		d1 := fpr - fnr
		if d0 == 0 {
			return (prevFPR + prevFNR) / 2
		}
		if d0 < 0 && d1 >= 0 {
			mix := d0 / (d0 - d1)
			fprAt := prevFPR + mix*(fpr-prevFPR)
			fnrAt := prevFNR + mix*(fnr-prevFNR)
			return (fprAt + fnrAt) / 2
		}
		prevFPR, prevFNR = fpr, fnr
	}
	return (prevFPR + prevFNR) / 2
}

// evalThresholdAtFA is the lowest finite threshold whose false-accept rate is at most maxFA.
// The second return is the achieved false-accept rate.
func evalThresholdAtFA(nontarget []float64, maxFA float64) (float64, float64) {
	n := len(nontarget)
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	scores := append([]float64(nil), nontarget...)
	sort.Float64s(scores)
	allowed := int(math.Floor(maxFA*float64(n) + 1e-12))
	if allowed >= n {
		return math.Inf(-1), 1
	}
	bestT := math.Inf(1)
	bestFA := 0.0
	for i := n - 1; i >= 0; {
		j := i
		for j > 0 && scores[j-1] == scores[i] {
			j--
		}
		accepted := n - j
		if accepted > allowed {
			break
		}
		bestT = scores[i]
		bestFA = float64(accepted) / float64(n)
		i = j - 1
	}
	if math.IsInf(bestT, 1) {
		return math.Nextafter(scores[n-1], math.Inf(1)), 0
	}
	return bestT, bestFA
}

func evalFalseReject(target []float64, threshold float64) float64 {
	if len(target) == 0 || math.IsNaN(threshold) {
		return math.NaN()
	}
	var rejected int
	for _, score := range target {
		if score < threshold {
			rejected++
		}
	}
	return float64(rejected) / float64(len(target))
}

func evalFalseAccept(nontarget []float64, threshold float64) float64 {
	if len(nontarget) == 0 || math.IsNaN(threshold) {
		return math.NaN()
	}
	var accepted int
	for _, score := range nontarget {
		if score >= threshold {
			accepted++
		}
	}
	return float64(accepted) / float64(len(nontarget))
}

func TestVoiceprintEvalMetrics(t *testing.T) {
	if math.Abs(evalEER([]float64{1, 1}, []float64{0, 0})) > 1e-9 {
		t.Fatal("separated EER")
	}
	if math.Abs(evalEER([]float64{0, 1}, []float64{0, 1})-0.5) > 1e-9 {
		t.Fatal("chance EER")
	}
	nontarget := make([]float64, 100)
	for i := range nontarget {
		nontarget[i] = float64(i)
	}
	threshold, far := evalThresholdAtFA(nontarget, 0.01)
	if threshold != 99 || math.Abs(far-0.01) > 1e-12 {
		t.Fatalf("threshold %v far %v", threshold, far)
	}
	if math.Abs(evalFalseReject([]float64{50, 100}, threshold)-0.5) > 1e-12 {
		t.Fatal("false reject")
	}
	if math.Abs(evalChunkScore([]float32{0.4, 0.4, 0.4})-0.4) > 1e-6 {
		t.Fatal("constant chunk score")
	}
}
