package voiceprint

import (
	"path/filepath"
	"testing"
)

func TestAdaptationProgressCountsClipsNotWindowsAndPersists(t *testing.T) {
	m, ctx, permit, profile := learningPermit(t)
	wins, scores := confidentRun(0.9, 4)
	silent := &Reference{Samples: make([]float32, 16), Complete: true}
	m.learnAdaptation(permit, profile, ctx, false, silent, wins, scores, nil, 48000, 16000, false)
	count, progress := m.LearningProgress()
	if count <= 1 || progress == nil || progress.LearnedClips != 1 || progress.AssistedClips != 0 {
		t.Fatalf("one clip with multiple windows was not counted once: %d %+v", count, progress)
	}
	// A clip can benefit from existing samples without qualifying for learning.
	m.learnAdaptation(permit, profile, ctx, false, nil, wins, scores, nil, 48000, 16000, true)
	root := filepath.Dir(filepath.Dir(m.path))
	reloaded, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, progress = reloaded.LearningProgress()
	if progress == nil || progress.LearnedClips != 1 || progress.AssistedClips != 1 {
		t.Fatalf("lost saved progress: %+v", progress)
	}
	progress.LearnedClips = 999
	copy := reloaded.Snapshot()
	copy.AdaptationProgress.AssistedClips = 999
	_, fresh := reloaded.LearningProgress()
	if fresh.LearnedClips != 1 || fresh.AssistedClips != 1 {
		t.Fatal("progress exposes mutable profile memory")
	}
	m.SetAdaptationEnabled(false)
	m.learnAdaptation(permit, profile, ctx, false, silent, wins, scores, nil, 48000, 16000, true)
	_, fresh = m.LearningProgress()
	if fresh.LearnedClips != 1 || fresh.AssistedClips != 1 {
		t.Fatal("disabled adaptation changed progress")
	}
	if err := m.ClearAdaptation(); err != nil {
		t.Fatal(err)
	}
	count, fresh = m.LearningProgress()
	if count != 0 || fresh == nil || *fresh != (AdaptationProgress{}) || m.Snapshot().Embeddings[0][0] != 1 {
		t.Fatalf("clear failed to reset progress or preserve enrollment: %d %+v", count, fresh)
	}
}

func TestAdaptationProgressContinuesAfterSamplePoolIsFull(t *testing.T) {
	m, ctx, permit, profile := learningPermit(t)
	for range adaptationCap + 3 {
		m.storeAdaptation(permit, profile, ctx, testVectors())
	}
	count, progress := m.LearningProgress()
	if count != adaptationCap || progress.LearnedClips != adaptationCap+3 {
		t.Fatalf("cumulative clips were capped with samples: %d %+v", count, progress)
	}
}

func TestAdaptationAssistanceRequiresAnActualLabelChange(t *testing.T) {
	profile := enrolledV3(t).Snapshot()
	wins, _ := confidentRun(0.9, 4)
	for _, test := range []struct {
		name              string
		original, boosted float32
		blocked, want     bool
	}{
		{"newly confirmed speech", .4, .6, false, true},
		{"already confirmed", .6, .9, false, false},
		{"higher score without confirmation", .3, .4, false, false},
		{"unchanged score", .6, .6, false, false},
		{"playback veto", .4, .6, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			original, boosted := make([]float32, len(wins)), make([]float32, len(wins))
			blocked := make([]bool, len(wins))
			for i := range wins {
				original[i], boosted[i], blocked[i] = test.original, test.boosted, test.blocked
			}
			spans := labelScoredSpans(profile, wins, boosted, blocked, 48000, 16000, false)
			if got := adaptationAssisted(profile, wins, original, boosted, blocked, spans, 48000, 16000); got != test.want {
				t.Fatalf("assisted=%v want=%v spans=%+v", got, test.want, spans)
			}
		})
	}
	t.Run("partial confidence does not confirm the utterance", func(t *testing.T) {
		original := []float32{.1, .1, .1, .1, .1, .1}
		boosted := []float32{.6, .6, .6, .1, .1, .1}
		spans := labelScoredSpans(profile, wins, boosted, nil, 48000, 16000, false)
		if adaptationAssisted(profile, wins, original, boosted, nil, spans, 48000, 16000) {
			t.Fatalf("partial evidence counted as a confirmed utterance: %+v", spans)
		}
	})
}

func TestAdaptationProgressKeepsLegacyTotalsUnknownAndFailedWritesUncounted(t *testing.T) {
	m, ctx, permit, profile := learningPermit(t)
	if _, progress := m.LearningProgress(); progress != nil {
		t.Fatal("invented cumulative totals for an older profile")
	}
	m.storeAdaptation(permit, profile, ctx, testVectors())
	_, before := m.LearningProgress()
	blockProfileStorage(t, m)
	m.storeAdaptationProgress(permit, profile, ctx, testVectors(), true)
	_, after := m.LearningProgress()
	if *after != *before {
		t.Fatal("failed persistence advanced displayed progress")
	}
}
