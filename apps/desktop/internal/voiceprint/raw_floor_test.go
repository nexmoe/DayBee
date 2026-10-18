package voiceprint

import (
	"errors"
	"testing"
)

func TestRawFloorSurvivesSmoothingAndSustain(t *testing.T) {
	floor := []float32{10, asNormMin, 10}
	smoothed := smoothScores(floor)
	if smoothed[1] != asNormMin {
		t.Fatalf("median lifted the floor: %v", smoothed)
	}
	spike := smoothScores([]float32{0.1, 0.9, 0.1})
	for i, got := range spike {
		if got != 0.1 {
			t.Fatalf("ordinary median[%d] = %v", i, spike)
		}
	}
	windows := []WindowScore{{Start: 0}, {Start: 8000}, {Start: 16000}, {Start: 24000}}
	scores := []float32{5, asNormMin, 5, 5}
	echo := []bool{false, false, false, false}
	spans := IdentitySpans(windows, scores, echo, 32000, 16000, 0.2, 0.8)
	if echo[1] {
		t.Fatal("hard reject rewrote the caller echo mask")
	}
	for _, span := range spans {
		if span.Start < 16000 && span.End > 8000 && span.Identity == "me" {
			t.Fatalf("short floor rejection joined me: %+v", spans)
		}
	}
}

func TestDecisionScoresReapplyTheRawFloor(t *testing.T) {
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	profile := &Profile{
		Version: calibratedVersion, Model: ModelName, Device: "native:mic-1", Embeddings: testVectors(),
		Calibration: &Calibration{TLow: 0.2, THigh: 1, Model: ModelName, CohortID: meta.BlobSHA256},
	}
	emb := make([]float32, embeddingDimension)
	emb[2] = 1
	raw := ProfileScore(profile, emb)
	if !(raw < Threshold(ModelName)) {
		t.Fatalf("fixture raw %v already clears the floor", raw)
	}
	scored := DecisionScores(profile, []WindowScore{{Score: 20, Embedding: emb}})
	if len(scored) != 1 || scored[0] != asNormMin {
		t.Fatalf("high stored score bypassed the raw floor: %v", scored)
	}
	if identity, _ := DecideProfile(profile, scored, nil); identity == "me" {
		t.Fatal("raw rejection plus a high z became me")
	}
	legacy := &Profile{Version: 2, Model: ModelName, Embeddings: testVectors()}
	if got := DecisionScores(legacy, []WindowScore{{Score: raw, Embedding: emb}}); len(got) != 1 || got[0] != raw {
		t.Fatalf("legacy score changed: %v", got)
	}
}

func TestNegativeScoresUseTheSavedProfileOnce(t *testing.T) {
	cohort, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	third := append([]float32(nil), cohort[0]...)
	groups := [][][]float32{directionGroup(0, 2), directionGroup(0.05, 2), {third}}
	_, impostor, err := crossScores(groups, cohort, meta.Windows)
	if err != nil {
		t.Fatal(err)
	}
	if len(impostor) != len(cohort) {
		t.Fatalf("negatives %d, want one pass over the cohort", len(impostor))
	}
	saved := &Profile{Version: calibratedVersion, Model: ModelName, Embeddings: collectEmbeddings(groups)}
	refs, err := speakerReferences(cohort, meta.Windows)
	if err != nil {
		t.Fatal(err)
	}
	testSide, err := negativeTestNorms(cohort, meta.Windows, refs)
	if err != nil {
		t.Fatal(err)
	}
	enrollSide, err := negativeEnrollNorms(saved, meta.Windows, refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, vector := range cohort {
		stats := enrollSide[meta.Windows[i].Speaker]
		want, ok := gatedNorm(ProfileScore(saved, vector), stats.mean, stats.std, testSide[i].mean, testSide[i].std)
		if !ok || impostor[i] != want {
			t.Fatalf("index %d got %v want %v", i, impostor[i], want)
		}
	}
	fold := &Profile{Version: calibratedVersion, Model: ModelName, Embeddings: otherEmbeddings(groups, 2)}
	if ProfileScore(saved, cohort[0]) <= ProfileScore(fold, cohort[0]) {
		t.Fatal("final profile did not keep the third enrollment group")
	}
}

func TestTwoEnrollmentGroupsAreRejected(t *testing.T) {
	groups := [][][]float32{directionGroup(0, 2), directionGroup(0.01, 2)}
	if _, err := Calibrate(groups, EnrollmentContext{Device: "native:mic-1"}); !errors.Is(err, ErrEnrollmentSegments) {
		t.Fatal("two groups were accepted", err)
	}
}

func TestDecideProfileMeanBelowLowIsNotMe(t *testing.T) {
	profile := &Profile{Version: calibratedVersion, Model: ModelName,
		Calibration: &Calibration{TLow: 0.4, THigh: 0.8, Model: ModelName}}
	identity, mean := DecideProfile(profile, []float32{0.1, 0.5, 0.1, 0.5}, nil)
	if identity != "not_me" || !(mean < 0.4) {
		t.Fatalf("mean below low = %q %v", identity, mean)
	}
}

func TestIllegalBandCannotAcceptTheFloorSentinel(t *testing.T) {
	for _, band := range []Calibration{
		{TLow: -30, THigh: -10, Model: ModelName, CohortID: "x"},
		{TLow: asNormMin, THigh: 1, Model: ModelName, CohortID: "x"},
		{TLow: 0.2, THigh: asNormMax + 1, Model: ModelName, CohortID: "x"},
	} {
		if bandOK(&band) {
			t.Fatalf("accepted band %+v", band)
		}
		profile := &Profile{Version: calibratedVersion, Model: ModelName, Device: "native:mic-1", Embeddings: testVectors(), Calibration: &band}
		if calibrationOK(profile) {
			t.Fatalf("accepted profile %+v", band)
		}
		if identity, _ := DecideProfile(profile, []float32{asNormMin, 20}, nil); identity == "me" {
			t.Fatalf("band %+v labeled the sentinel me", band)
		}
		low, high := DecisionThresholds(profile)
		if !(high < low) {
			t.Fatalf("illegal band stayed usable: %v %v", low, high)
		}
	}
}
