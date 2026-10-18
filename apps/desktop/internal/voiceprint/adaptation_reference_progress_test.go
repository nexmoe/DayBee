package voiceprint

import "testing"

func TestAdaptationProgressWaitsForReferenceThenLearns(t *testing.T) {
	m, ctx, permit, profile := learningPermit(t)
	wins, scores := confidentRun(.9, 4)
	playback := make([]float32, 16)
	for i := range playback {
		playback[i] = .1
	}
	for _, ref := range []*Reference{
		nil,
		{Samples: make([]float32, 16), Complete: false},
		{Samples: playback, Complete: true},
	} {
		m.learnAdaptation(permit, profile, ctx, false, ref, wins, scores, nil, 48000, 16000, false)
		if count, progress := m.LearningProgress(); count != 0 || progress != nil {
			t.Fatal("unchecked playback advanced learning")
		}
	}
	m.learnAdaptation(permit, profile, ctx, false, &Reference{Samples: make([]float32, 16), Complete: true}, wins, scores, nil, 48000, 16000, false)
	if count, progress := m.LearningProgress(); count == 0 || progress == nil || progress.LearnedClips != 1 {
		t.Fatalf("learning did not resume after reference recovery: %d %+v", count, progress)
	}
}
