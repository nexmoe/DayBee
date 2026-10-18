package voiceprint

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestHysteresisBandStaysNearTheFalseAcceptPoint(t *testing.T) {
	low, high, err := hysteresisThresholds(0, 10)
	if err != nil || low != 0 || high != float32(0.3) {
		t.Fatalf("wide gap = %v %v %v", low, high, err)
	}
	low, high, err = hysteresisThresholds(1, 1.2)
	if err != nil || low != 1 || high != float32(1.1) {
		t.Fatalf("narrow gap = %v %v %v", low, high, err)
	}
	if _, _, err = hysteresisThresholds(2, 1); !errors.Is(err, ErrEnrollmentInseparable) {
		t.Fatal("inverted interval was accepted", err)
	}
	if _, _, err = hysteresisThresholds(1, 1); !errors.Is(err, ErrEnrollmentInseparable) {
		t.Fatal("zero gap was accepted", err)
	}
	if _, _, err = hysteresisThresholds(1, math.Nextafter(1, 2)); !errors.Is(err, ErrEnrollmentInseparable) {
		t.Fatal("float32-collapsed gap was accepted", err)
	}
}

func TestRateBoundsKeepFalseAcceptAndFalseReject(t *testing.T) {
	impostor := make([]float64, 100)
	for i := range impostor {
		impostor[i] = float64(i)
	}
	fa, ok := thresholdAtFA(impostor, 0.01)
	if !ok || fa != 99 || math.Abs(falseAccept(impostor, fa)-0.01) > 1e-12 {
		t.Fatalf("FA threshold %v", fa)
	}
	genuine := make([]float64, 100)
	for i := range genuine {
		genuine[i] = float64(i)
	}
	fr, ok := thresholdFRUpper(genuine, 0.05)
	if !ok || fr != 5 || math.Abs(falseReject(genuine, fr)-0.05) > 1e-12 {
		t.Fatalf("FR upper %v", fr)
	}
	if _, _, err := rateBounds([]float64{0, 0, 0, 0}, []float64{1, 1, 1, 1}); !errors.Is(err, ErrEnrollmentInseparable) {
		t.Fatal("overlapping enrollment was accepted", err)
	}
	separatedGenuine := []float64{100, 100, 100, 100}
	fa, fr, err := rateBounds(separatedGenuine, impostor)
	if err != nil || !(fa <= fr) {
		t.Fatal("separated scores were rejected", fa, fr, err)
	}
	low, high, err := hysteresisThresholds(fa, fr)
	if err != nil || !(high > low) || high-low > float32(hysteresisCap)+1e-4 {
		t.Fatalf("band %v %v %v", low, high, err)
	}
	if falseAccept(impostor, float64(low)) > 0.01+1e-12 || falseReject(separatedGenuine, float64(high)) > 0.05+1e-12 {
		t.Fatal("chosen thresholds missed the rate limits")
	}
}

func TestNormTopIsFiniteWhenScoresDoNotVary(t *testing.T) {
	mean, std, ok := normTop([]float64{0.2, 0.2, 0.2})
	if !ok || math.Abs(mean-0.2) > 1e-12 || std != asNormSigmaFloor {
		t.Fatalf("flat norm %v %v %v", mean, std, ok)
	}
	value := asNorm(1, mean, std, mean, std)
	if !finite32(value) || value < asNormMin || value > asNormMax {
		t.Fatal("normalized score was not bounded", value)
	}
}

func TestCalibrateRejectsUnresolvedDevicesAndSingleSegment(t *testing.T) {
	groups := [][][]float32{directionGroup(0, 4), directionGroup(0, 4)}
	for _, device := range []string{"", "   ", "default", " default "} {
		if _, err := Calibrate(groups, EnrollmentContext{Device: device}); !errors.Is(err, ErrEnrollmentDevice) {
			t.Fatalf("%q was accepted: %v", device, err)
		}
	}
	if _, err := Calibrate(groups[:1], EnrollmentContext{Device: "native:mic-1"}); !errors.Is(err, ErrEnrollmentSegments) {
		t.Fatal("one segment was accepted", err)
	}
}

func TestCalibrateSeparatesStableSegmentsFromThePublicCohort(t *testing.T) {
	groups := [][][]float32{directionGroup(0, 4), directionGroup(0.01, 4), directionGroup(0.02, 4)}
	profile, err := Calibrate(groups, EnrollmentContext{Device: " native:mic-1 ", EchoCancellation: true})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Version != calibratedVersion || profile.Device != "native:mic-1" || !profile.EchoCancellation {
		t.Fatalf("profile context %+v", profile)
	}
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Calibration == nil || profile.Calibration.Model != ModelName || profile.Calibration.CohortID != meta.BlobSHA256 || !(profile.Calibration.THigh > profile.Calibration.TLow) {
		t.Fatal("missing band", profile.Calibration)
	}
	if profile.Calibration.THigh-profile.Calibration.TLow > float32(hysteresisCap)+1e-4 {
		t.Fatal("band wider than 0.3", profile.Calibration)
	}
	low, high := DecisionThresholds(profile)
	if low != profile.Calibration.TLow || high != profile.Calibration.THigh {
		t.Fatal("thresholds diverged from calibration")
	}
	raw := ProfileScore(profile, groups[0][0])
	scored := DecisionScores(profile, []WindowScore{{Score: raw, Embedding: groups[0][0]}})
	if len(scored) != 1 || !finite32(scored[0]) || scored[0] == raw {
		t.Fatalf("decision score %v raw %v", scored, raw)
	}
	legacy := &Profile{Version: 2, Model: ModelName, Embeddings: profile.Embeddings}
	legacyScores := DecisionScores(legacy, []WindowScore{{Score: raw, Embedding: groups[0][0]}})
	if len(legacyScores) != 1 || legacyScores[0] != raw {
		t.Fatal("legacy score was normalized", legacyScores, raw)
	}
	legacyLow, legacyHigh := DecisionThresholds(legacy)
	if legacyLow != Threshold(ModelName) || legacyHigh != legacyLow {
		t.Fatal("legacy thresholds changed", legacyLow, legacyHigh)
	}
}

func TestCalibrateRejectsMixedCohortSpeakers(t *testing.T) {
	cohort, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	groups := make([][][]float32, 3)
	for speaker := 0; speaker < 3; speaker++ {
		base := speaker * cohortPerSpeaker
		groups[speaker] = append([][]float32(nil), cohort[base:base+4]...)
	}
	if meta.Speakers[0] == meta.Speakers[1] {
		t.Fatal("cohort speakers were not distinct")
	}
	if _, err := Calibrate(groups, EnrollmentContext{Device: "native:mic-1"}); !errors.Is(err, ErrEnrollmentInseparable) {
		t.Fatal("mixed public speakers were enrolled", err)
	}
}

func TestLegacyProfileStaysCompatibleWhenDeviceIsUnresolved(t *testing.T) {
	legacy := &Profile{Version: 1, Model: ModelName, Embeddings: testVectors()}
	if !Compatible(legacy, EnrollmentContext{Device: "default"}) || !Compatible(legacy, EnrollmentContext{}) {
		t.Fatal("legacy profile was bound to a device")
	}
	enrolled := &Profile{
		Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(), Device: "native:mic-1",
		Calibration: &Calibration{TLow: 0.2, THigh: 0.5, Model: ModelName, CohortID: "compat-cohort"},
	}
	if Compatible(enrolled, EnrollmentContext{Device: "default", EchoCancellation: false}) {
		t.Fatal("default matched a resolved profile")
	}
	if Compatible(enrolled, EnrollmentContext{Device: "native:other"}) {
		t.Fatal("a different microphone matched")
	}
	if !Compatible(enrolled, EnrollmentContext{Device: "native:mic-1"}) {
		t.Fatal("the enrolled microphone did not match")
	}
	enrolled.EchoCancellation = true
	if Compatible(enrolled, EnrollmentContext{Device: "native:mic-1", EchoCancellation: false}) {
		t.Fatal("echo setting was ignored")
	}
}

func directionGroup(offset float32, n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		vector := make([]float32, embeddingDimension)
		vector[0] = 1
		vector[1] = offset + float32(i)*0.0001
		out[i] = vector
	}
	return out
}

func TestCalibrationJSONUsesContractNames(t *testing.T) {
	raw, err := json.Marshal(Calibration{TLow: 0.25, THigh: 0.55, Model: ModelName, CohortID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tLow", "tHigh", "model", "cohortId"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing %s in %s", key, raw)
		}
	}
}

func TestBrokenV3DoesNotAcceptRawMe(t *testing.T) {
	raw := float32(0.99)
	window := []WindowScore{{Score: raw, Embedding: testVectors()[0]}}
	legacyThreshold := Threshold(ModelName)
	damaged := []*Profile{
		{Version: calibratedVersion, Model: ModelName, Embeddings: testVectors()},
		{Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(), Device: "native:mic-1",
			Calibration: &Calibration{TLow: 0.9, THigh: 0.2, Model: ModelName, CohortID: "x"}},
	}
	for _, profile := range damaged {
		assertRawCannotBeMe(t, profile, window, raw, legacyThreshold)
		if identity, _ := DecideProfile(profile, []float32{raw}, nil); identity != "uncertain" {
			t.Fatalf("damaged raw 0.99 decided %q", identity)
		}
	}
	mismatched := []*Profile{
		{Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(), Device: "native:mic-1",
			Calibration: &Calibration{TLow: 0.2, THigh: 0.5, Model: ModelName, CohortID: "not-the-bundled-cohort"}},
		{Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(), Device: "default",
			Calibration: &Calibration{TLow: 0.2, THigh: 0.5, Model: ModelName, CohortID: "x"}},
	}
	for _, profile := range mismatched {
		assertRawCannotBeMe(t, profile, window, raw, legacyThreshold)
		low, high := DecisionThresholds(profile)
		if low != profile.Calibration.TLow || high != profile.Calibration.THigh {
			t.Fatalf("stored band changed: %v %v", low, high)
		}
	}
}

func assertRawCannotBeMe(t *testing.T, profile *Profile, window []WindowScore, raw, legacyThreshold float32) {
	t.Helper()
	scored := DecisionScores(profile, window)
	if len(scored) != 1 || scored[0] == raw {
		t.Fatalf("leaked raw %v", scored)
	}
	low, high := DecisionThresholds(profile)
	if low == legacyThreshold && high == legacyThreshold {
		t.Fatal("broken v3 used the legacy cosine threshold")
	}
	if identity, _ := DecideProfile(profile, scored, nil); identity == "me" {
		t.Fatal("raw 0.99 became me")
	}
	if identity, _ := chunkLabel(profile, window, nil); identity != "" {
		t.Fatalf("Match label %q", identity)
	}
}

func TestLegacyProfilesKeepRawScores(t *testing.T) {
	raw := float32(0.99)
	window := []WindowScore{{Score: raw, Embedding: testVectors()[0]}}
	legacyThreshold := Threshold(ModelName)
	for _, version := range []int{1, 2} {
		profile := &Profile{Version: version, Model: ModelName, Embeddings: testVectors()}
		if scored := DecisionScores(profile, window); len(scored) != 1 || scored[0] != raw {
			t.Fatalf("v%d score %v", version, scored)
		}
		low, high := DecisionThresholds(profile)
		if low != legacyThreshold || high != legacyThreshold {
			t.Fatalf("v%d thresholds %v %v", version, low, high)
		}
		if identity, _ := DecideProfile(profile, []float32{raw}, nil); identity != "me" {
			t.Fatalf("v%d raw was %q", version, identity)
		}
		if identity, _ := chunkLabel(profile, window, nil); identity != "me" {
			t.Fatalf("v%d Match was %q", version, identity)
		}
	}
}

func TestTiedImpostorsStayBelowTheFalseAcceptBoundary(t *testing.T) {
	impostor := make([]float64, 40)
	genuine := make([]float64, 40)
	for i := range genuine {
		genuine[i] = 1
	}
	fa, ok := thresholdAtFA(impostor, acceptFalseRate)
	if !ok || !finite32(float32(fa)) || !(float64(float32(fa)) > 0) {
		t.Fatalf("tied FA boundary %v ok %v", fa, ok)
	}
	if falseAccept(impostor, float64(float32(fa))) > acceptFalseRate+1e-12 {
		t.Fatal("float32 boundary still accepts the tied impostors")
	}
	_, _, err := rateBounds(genuine, impostor)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNegativeTrialNormOmitsQuerySpeaker(t *testing.T) {
	cohort, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	refs, err := speakerReferences(cohort, meta.Windows)
	if err != nil || len(refs) != len(cohortSpeakerIDs) {
		t.Fatal(err, len(refs))
	}
	for _, speaker := range cohortSpeakerIDs {
		ref := refs[speaker]
		if len(ref) != len(cohort)-cohortPerSpeaker {
			t.Fatalf("%s reference %d", speaker, len(ref))
		}
		dropped := 0
		for _, window := range meta.Windows {
			if window.Speaker == speaker {
				dropped++
			}
		}
		if dropped != cohortPerSpeaker {
			t.Fatalf("%s dropped %d", speaker, dropped)
		}
	}
	model := &Profile{Version: calibratedVersion, Model: ModelName, Embeddings: directionGroup(0, 3)}
	for _, index := range []int{0, cohortPerSpeaker, len(cohort) - 1} {
		enroll, test, err := negativeTrialNorms(model, cohort, meta.Windows, index)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := referenceWithoutSpeaker(cohort, meta.Windows, meta.Windows[index].Speaker)
		if err != nil {
			t.Fatal(err)
		}
		wantEnroll, wantEnrollStd, ok := normTop(cohortProfileScores(model, ref))
		wantTest, wantTestStd, testOK := normTop(cohortCosines(cohort[index], ref))
		if !ok || !testOK || enroll.mean != wantEnroll || enroll.std != wantEnrollStd || test.mean != wantTest || test.std != wantTestStd {
			t.Fatalf("index %d norms did not use the excluded reference", index)
		}
		fullMean, fullStd, fullOK := normTop(cohortCosines(cohort[index], cohort))
		if !fullOK || (test.mean == fullMean && test.std == fullStd) {
			t.Fatalf("index %d test norm still includes the query speaker", index)
		}
	}
}
