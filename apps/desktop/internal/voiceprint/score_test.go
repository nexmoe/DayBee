package voiceprint

import (
	"math"
	"testing"
)

func unit(index int) []float32 {
	vector := make([]float32, embeddingDimension)
	vector[index] = 1
	return vector
}

func TestCosine(t *testing.T) {
	same := unit(0)
	if got := Cosine(same, append([]float32(nil), same...)); math.Abs(float64(got-1)) > 1e-6 {
		t.Fatalf("identical vectors scored %v", got)
	}
	if got := Cosine(same, unit(1)); got != 0 {
		t.Fatalf("orthogonal vectors scored %v", got)
	}
	opposite := unit(0)
	opposite[0] = -1
	if got := Cosine(same, opposite); math.Abs(float64(got+1)) > 1e-6 {
		t.Fatalf("opposite vectors scored %v", got)
	}
	if Cosine(same, []float32{1}) != 0 || Cosine(same, make([]float32, embeddingDimension)) != 0 || Cosine(nil, nil) != 0 {
		t.Fatal("degenerate vectors must score zero")
	}
}

func TestProfileScoreUsesCentroidOrTopThree(t *testing.T) {
	query := unit(0)
	spread := &Profile{Embeddings: [][]float32{unit(0), unit(1), unit(2), unit(3)}}
	if got := ProfileScore(spread, query); math.Abs(float64(got-0.5)) > 1e-5 {
		t.Fatalf("centroid score = %v, want 0.5", got)
	}
	repeated := &Profile{Embeddings: [][]float32{unit(1), query, query, query}}
	if got := ProfileScore(repeated, query); got <= ProfileScore(spread, query) {
		t.Fatalf("top-3 mean did not outrank a spread centroid: %v", got)
	}
	if math.Abs(float64(ProfileScore(repeated, query)-1)) > 1e-5 {
		t.Fatalf("top-3 of exact matches = %v", ProfileScore(repeated, query))
	}
	only := &Profile{Embeddings: [][]float32{query}}
	if math.Abs(float64(ProfileScore(only, query)-1)) > 1e-5 {
		t.Fatal("a single enrollment vector must match itself")
	}
	if ProfileScore(nil, query) != 0 || ProfileScore(&Profile{}, query) != 0 || ProfileScore(only, nil) != 0 {
		t.Fatal("missing profile or query scored as a match")
	}
}

func TestWindowSpans(t *testing.T) {
	short, err := windowSpans(minWindowSamples-1, voiceSampleRate)
	if err == nil || short != nil {
		t.Fatal("audio under 0.8 s was windowed")
	}
	if _, err := windowSpans(windowSamples, 8000); err == nil {
		t.Fatal("non-16 kHz audio was windowed")
	}
	one, err := windowSpans(minWindowSamples, voiceSampleRate)
	if err != nil || len(one) != 1 || one[0] != [2]int{0, minWindowSamples} {
		t.Fatalf("0.8 s window = %v %v", one, err)
	}
	whole, err := windowSpans(windowSamples, voiceSampleRate)
	if err != nil || len(whole) != 1 || whole[0] != [2]int{0, windowSamples} {
		t.Fatalf("1.5 s window = %v %v", whole, err)
	}
	trimmed, err := windowSpans(windowSamples+1, voiceSampleRate)
	if err != nil || len(trimmed) != 1 || trimmed[0] != [2]int{0, windowSamples} {
		t.Fatalf("just over 1.5 s = %v %v", trimmed, err)
	}
	// 2 s holds one 1.5 s window. 3 s holds the next window one hop later.
	oneHop, err := windowSpans(voiceSampleRate*2, voiceSampleRate)
	if err != nil || len(oneHop) != 1 || oneHop[0] != [2]int{0, windowSamples} {
		t.Fatalf("2 s windows = %v %v", oneHop, err)
	}
	two, err := windowSpans(voiceSampleRate*3, voiceSampleRate)
	if err != nil || len(two) != 2 || two[0] != [2]int{0, windowSamples} || two[1] != [2]int{hopSamples, hopSamples + windowSamples} {
		t.Fatalf("3 s windows = %v %v", two, err)
	}
}

func TestEmbedWindowsRejectsShortAudioBeforeTheModel(t *testing.T) {
	m, err := New(t.TempDir(), func() string { return "/missing/model.onnx" })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.EmbedWindows(make([]float32, minWindowSamples-1), voiceSampleRate); err == nil {
		t.Fatal("short audio reached the model")
	}
	if _, err := m.Score(&Profile{Embeddings: testVectors()}, make([]float32, minWindowSamples-1), voiceSampleRate); err == nil {
		t.Fatal("short audio was scored")
	}
	if identity, score := m.Match(&Profile{Embeddings: testVectors()}, make([]float32, 1000), voiceSampleRate, &Reference{Samples: []float32{1}}); identity != "" || score != 0 {
		t.Fatal("short audio was labeled")
	}
}
