package voiceprint

import "testing"

func TestRelaxHighStaysInsideTheBand(t *testing.T) {
	if got := relaxHigh(0.2, 0.8); got != 0.5 {
		t.Fatalf("wide band high = %v", got)
	}
	if got := relaxHigh(0.4, 0.5); got != 0.45 {
		t.Fatalf("narrow band high = %v", got)
	}
	if got := relaxHigh(0.7, 0.7); got != 0.7 {
		t.Fatalf("flat band changed: %v", got)
	}
	if got := relaxHigh(0.8, 0.2); got != 0.2 {
		t.Fatalf("inverted band changed: %v", got)
	}
}

func TestRelaxationSavesBoundaryWithoutPropagation(t *testing.T) {
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	profile := &Profile{Version: 3, Model: ModelName, Device: "mic-1",
		Calibration: &Calibration{TLow: 0.2, THigh: 0.8, Model: ModelName, CohortID: meta.BlobSHA256}}
	windows := []WindowScore{{Start: 0}, {Start: 8000}, {Start: 16000}, {Start: 24000}}
	boundary := []float32{0.6, 0.6, 0.6, 0.6}
	original := profile.Calibration
	originalHigh := original.THigh

	rescued := labelScoredSpans(profile, windows, boundary, nil, 32000, 16000, true)
	if profile.Calibration != original || profile.Calibration.THigh != originalHigh {
		t.Fatalf("relaxation mutated the profile: %+v", profile.Calibration)
	}
	if len(rescued) != 1 || rescued[0].Identity != "me" || rescued[0].Start != 0 || rescued[0].End != 32000 {
		t.Fatalf("boundary was not saved: %+v", rescued)
	}
	if rescued[0].Anchor {
		t.Fatal("rescued me can propagate")
	}
	plain := labelScoredSpans(profile, windows, boundary, nil, 32000, 16000, false)
	if len(plain) != 1 || plain[0].Identity == "me" || plain[0].Anchor {
		t.Fatalf("boundary passed the original high: %+v", plain)
	}

	strong := []float32{0.95, 0.95, 0.95, 0.95}
	kept := labelScoredSpans(profile, windows, strong, nil, 32000, 16000, true)
	if len(kept) != 1 || kept[0].Identity != "me" || !kept[0].Anchor {
		t.Fatalf("score above the original high lost its anchor: %+v", kept)
	}
	if profile.Calibration.THigh != originalHigh {
		t.Fatal("strong me mutated the profile")
	}
}

func TestMatchSpansShortAudioStaysUncertain(t *testing.T) {
	samples := make([]float32, 1000)
	spans := (&Manager{}).MatchSpans(&Profile{Embeddings: [][]float32{{1, 0}}}, samples, voiceSampleRate, &Reference{Samples: []float32{1}})
	if len(spans) != 1 || spans[0].Start != 0 || spans[0].End != len(samples) || spans[0].Identity != spanUncertain {
		t.Fatalf("short audio: %+v", spans)
	}
	if (&Manager{}).MatchSpans(nil, nil, voiceSampleRate, nil) != nil {
		t.Fatal("empty audio was labeled")
	}
}
