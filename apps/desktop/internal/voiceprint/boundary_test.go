package voiceprint

import "testing"

func TestBoundaryGuardUsesWindowSupport(t *testing.T) {
	profile := boundaryProfile(t)
	narrow := boundaryCase(t, profile, []WindowScore{{Start: 0, End: 7000}, {Start: 4000, End: 20000}}, []float32{0.1, 0.95}, nil, 20000)
	if !spanIs(narrow, 0, 0, 4000, spanNotMe) || !spanIs(narrow, 1, 4000, 7000, spanUncertain) || !spanIs(narrow, 2, 7000, 20000, spanMe) {
		t.Fatalf("narrow support: %+v", narrow)
	}
	if narrow[1].Anchor || !narrow[2].Anchor {
		t.Fatalf("anchor after the split: %+v", narrow)
	}
	wide := boundaryCase(t, profile, []WindowScore{{Start: 0, End: 9000}, {Start: 2500, End: 20000}}, []float32{0.1, 0.95}, nil, 20000)
	if !spanIs(wide, 1, 2500, 9000, spanUncertain) || wide[1].End-wide[1].Start == narrow[1].End-narrow[1].Start {
		t.Fatalf("guard followed a fixed delay: narrow %+v wide %+v", narrow, wide)
	}
	if !spanIs(wide, 0, 0, 2500, spanNotMe) || !spanIs(wide, 2, 9000, 20000, spanMe) {
		t.Fatalf("wide support: %+v", wide)
	}
}

func TestBoundaryGuardScoreStaysWeighted(t *testing.T) {
	profile := boundaryProfile(t)
	wins := []WindowScore{{Start: 0, End: 20000}, {Start: 16000, End: 32000}, {Start: 32000, End: 48000}}
	scores := []float32{0.85, 0.99, 0.85}
	spans := boundaryCase(t, profile, wins, scores, []bool{true, false, false}, 48000)
	if !spanIs(spans, 2, 20000, 48000, spanMe) || !spans[2].Anchor {
		t.Fatalf("split: %+v", spans)
	}
	want := ownedScore(wins, smoothScores(scores), 48000, 20000, 48000)
	if spans[2].Score != want || want == 0.99 {
		t.Fatalf("vote mean replaced the weighted score: got %v want %v", spans[2].Score, want)
	}
}

func TestBoundaryGuardIgnoresEarlierWindow(t *testing.T) {
	profile := boundaryProfile(t)
	spans := boundaryCase(t, profile, []WindowScore{
		{Start: 0, End: 100000},
		{Start: 5000, End: 6000},
		{Start: 10000, End: 26000},
	}, []float32{0.1, 0.1, 0.95}, nil, 26000)
	if len(spans) != 2 || !spanIs(spans, 0, 0, 10000, spanNotMe) || !spanIs(spans, 1, 10000, 26000, spanMe) {
		t.Fatalf("earlier window expanded the cut: %+v", spans)
	}
}

func TestBoundaryGuardKeepsEchoFloorAndUniformSpans(t *testing.T) {
	profile := boundaryProfile(t)
	echoed := boundaryCase(t, profile, []WindowScore{{Start: 0, End: 9000}, {Start: 4000, End: 20000}}, []float32{0.99, 0.95}, []bool{true, false}, 20000)
	if !spanIs(echoed, 0, 0, 4000, spanNotMe) || !spanIs(echoed, 1, 4000, 9000, spanUncertain) || echoed[0].Identity == spanMe {
		t.Fatalf("echo became me: %+v", echoed)
	}
	floored := boundaryCase(t, profile, []WindowScore{{Start: 0, End: 8000}, {Start: 3000, End: 20000}}, []float32{asNormMin, 0.95}, nil, 20000)
	if !spanIs(floored, 0, 0, 3000, spanNotMe) || !spanIs(floored, 1, 3000, 8000, spanUncertain) {
		t.Fatalf("floor: %+v", floored)
	}
	same := boundaryCase(t, profile, []WindowScore{{Start: 0, End: 9000}, {Start: 4000, End: 12000}}, []float32{0.95, 0.96}, nil, 12000)
	if len(same) != 1 || !spanIs(same, 0, 0, 12000, spanMe) || !same[0].Anchor {
		t.Fatalf("uniform me changed: %+v", same)
	}
	legacy := boundaryCase(t, &Profile{Version: 2, Model: ModelName}, []WindowScore{{Start: 0, End: 7000}, {Start: 4000, End: 20000}}, []float32{0.1, 0.95}, nil, 20000)
	if !spanIs(legacy, 0, 0, 4000, spanNotMe) || !spanIs(legacy, 1, 4000, 7000, spanUncertain) || !spanIs(legacy, 2, 7000, 20000, spanMe) {
		t.Fatalf("legacy: %+v", legacy)
	}
}

func TestBoundaryGuardShortTailStaysCovered(t *testing.T) {
	profile := boundaryProfile(t)
	wins := []WindowScore{{Start: 0, End: 50000}, {Start: 16000, End: 32000}}
	spans := boundaryCase(t, profile, wins, []float32{0.1, 0.95}, nil, 32000)
	if !spanIs(spans, 0, 0, 16000, spanNotMe) || !spanIs(spans, 1, 16000, 32000, spanUncertain) {
		t.Fatalf("short tail: %+v", spans)
	}
	if spans[0].End != spans[1].Start || spans[len(spans)-1].End != 32000 {
		t.Fatalf("coverage: %+v", spans)
	}
	plain := IdentitySpans(wins, []float32{0.1, 0.95}, nil, 32000, voiceSampleRate, 0.2, 0.8)
	if len(plain) != 2 || plain[1].Identity != spanMe || plain[1].Start != 16000 {
		t.Fatalf("IdentitySpans itself was guarded: %+v", plain)
	}
}

func TestGuardRegionIsNotLearned(t *testing.T) {
	profile := boundaryProfile(t)
	emb := func(seed float32) []float32 {
		out := make([]float32, embeddingDimension)
		for i := range out {
			out[i] = seed
		}
		return out
	}
	overlap := emb(1)
	kept := emb(2)
	wins := []WindowScore{
		{Start: 0, End: 24000, Score: 0.9, Embedding: emb(9)},
		{Start: 16000, End: 40000, Score: 0.9, Embedding: overlap},
		{Start: 32000, End: 56000, Score: 0.9, Embedding: kept},
		{Start: 48000, End: 64000, Score: 0.9, Embedding: emb(3)},
	}
	scores := []float32{5, 5, 5, 5}
	echo := []bool{true, false, false, false}
	got := adaptationCandidates(profile, wins, scores, echo, 64000, voiceSampleRate)
	for _, vector := range got {
		if vector[0] == overlap[0] {
			t.Fatal("window overlapping the guard region was learned")
		}
	}
	if len(got) == 0 || got[0][0] != kept[0] {
		t.Fatalf("fully contained me window was dropped: %d", len(got))
	}
}

func boundaryProfile(t *testing.T) *Profile {
	t.Helper()
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	return &Profile{Version: 3, Model: ModelName, Device: "mic-1",
		Calibration: &Calibration{TLow: 0.2, THigh: 0.8, Model: ModelName, CohortID: meta.BlobSHA256}}
}

func boundaryCase(t *testing.T, profile *Profile, wins []WindowScore, scores []float32, echo []bool, n int) []IdentitySpan {
	t.Helper()
	return labelScoredSpans(profile, wins, scores, echo, n, voiceSampleRate, false)
}

func spanIs(spans []IdentitySpan, i, start, end int, identity string) bool {
	return i < len(spans) && spans[i].Start == start && spans[i].End == end && spans[i].Identity == identity
}
