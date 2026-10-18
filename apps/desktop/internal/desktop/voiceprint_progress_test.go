package desktop

import (
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func TestVoiceprintSnapshotExposesPersistedLearningProgress(t *testing.T) {
	s, manager, _ := adaptationService(t)
	profile := adaptationProfile(2)
	profile.AdaptationProgress = &voiceprint.AdaptationProgress{LearnedClips: 47, AssistedClips: 8}
	if err := manager.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	snapshot := s.voiceprintSnapshot()
	if snapshot.Adaptation == nil || snapshot.Adaptation.LearnedClips != 47 || snapshot.Adaptation.AssistedClips != 8 || snapshot.AdaptationCount != 1 {
		t.Fatalf("wrong progress snapshot: %+v", snapshot)
	}
	if _, err := s.ClearVoiceprintAdaptation(); err != nil {
		t.Fatal(err)
	}
	snapshot = s.voiceprintSnapshot()
	if snapshot.Adaptation == nil || snapshot.Adaptation.LearnedClips != 0 || snapshot.Adaptation.AssistedClips != 0 || !manager.Enrolled() {
		t.Fatalf("reset did not update counters or kept no voiceprint: %+v", snapshot)
	}
}
