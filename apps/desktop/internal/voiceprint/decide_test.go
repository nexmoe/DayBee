package voiceprint

import "testing"

func TestMedianSmoothingRemovesSpikes(t *testing.T) {
	spike := smoothScores([]float32{0.1, 0.9, 0.1})
	for i, got := range spike {
		if got != 0.1 {
			t.Fatalf("spike[%d] = %v", i, spike)
		}
	}
	dip := smoothScores([]float32{0.9, 0.1, 0.9})
	for i, got := range dip {
		if got != 0.9 {
			t.Fatalf("dip[%d] = %v", i, dip)
		}
	}
	if single := smoothScores([]float32{0.4}); len(single) != 1 || single[0] != 0.4 {
		t.Fatalf("single window changed: %v", single)
	}
	edges := smoothScores([]float32{0.2, 0.8})
	if edges[0] != 0.2 || edges[1] != 0.8 {
		t.Fatalf("two-point median = %v", edges)
	}
}

func TestDecideRequiresFractionAndMean(t *testing.T) {
	high := make([]float32, 10)
	for i := range high {
		high[i] = 0.8
	}
	if identity, mean := Decide(high, nil, ModelName); identity != "me" || mean != 0.8 {
		t.Fatalf("confident speech = %q %v", identity, mean)
	}
	low := make([]float32, 10)
	for i := range low {
		low[i] = 0.4
	}
	if identity, mean := Decide(low, nil, ModelName); identity != "" || mean != 0.4 {
		t.Fatalf("other speaker = %q %v", identity, mean)
	}
	mixed := make([]float32, 10)
	for i := range mixed {
		mixed[i] = 0.8
		if i >= 7 {
			mixed[i] = 0.1
		}
	}
	if identity, _ := Decide(mixed, nil, ModelName); identity != "me" {
		t.Fatal("seven of ten windows should pass")
	}
	weak := make([]float32, 10)
	for i := range weak {
		weak[i] = 0.55
		if i >= 7 {
			weak[i] = 0
		}
	}
	if identity, mean := Decide(weak, nil, ModelName); identity != "" || mean >= Threshold(ModelName) {
		t.Fatalf("low mean was accepted: %q %v", identity, mean)
	}
	if identity, mean := Decide(nil, nil, ModelName); identity != "" || mean != 0 {
		t.Fatal("empty scores were labeled")
	}
	if Threshold("unknown-model") != 0.55 || Threshold(ModelName) != 0.537 {
		t.Fatal("CAM++ threshold changed")
	}
}

func TestDecideCountsEchoAsNotMe(t *testing.T) {
	scores := make([]float32, 10)
	for i := range scores {
		scores[i] = 0.9
	}
	echo := make([]bool, 10)
	for i := 0; i < 4; i++ {
		echo[i] = true
	}
	if identity, mean := Decide(scores, echo, ModelName); identity != "" || mean != 0.9 {
		t.Fatalf("four echo windows were accepted: %q %v", identity, mean)
	}
	echo[3] = false
	if identity, _ := Decide(scores, echo, ModelName); identity != "me" {
		t.Fatal("three echo windows should still leave a 0.7 me fraction")
	}
	for i := range echo {
		echo[i] = true
	}
	if identity, _ := Decide(scores, echo, ModelName); identity != "" {
		t.Fatal("an all-echo chunk was labeled me")
	}
}
