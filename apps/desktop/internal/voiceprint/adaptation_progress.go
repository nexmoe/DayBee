package voiceprint

// AdaptationProgress counts accepted audio clips, not embedding windows or
// sentences. Assistance means a whole clip became "me" versus the exact same
// decision without adaptation; it is not an estimate of recognition accuracy.
type AdaptationProgress struct {
	LearnedClips  uint64 `json:"learnedClips"`
	AssistedClips uint64 `json:"assistedClips"`
}

// LearningProgress returns a copy without exposing speaker embeddings. A nil
// progress distinguishes older profiles whose cumulative totals are unknown.
func (m *Manager) LearningProgress() (int, *AdaptationProgress) {
	if m == nil {
		return 0, nil
	}
	m.profileMu.RLock()
	defer m.profileMu.RUnlock()
	if m.profile == nil {
		return 0, nil
	}
	if m.profile.AdaptationProgress == nil {
		return len(m.profile.Adaptation), nil
	}
	progress := *m.profile.AdaptationProgress
	return len(m.profile.Adaptation), &progress
}

func adaptationAssisted(p *Profile, wins []WindowScore, original, boosted []float32, blocked []bool, spans []IdentitySpan, n, rate int) bool {
	clip := WindowScore{Start: 0, End: n}
	if !windowInsideMe(clip, spans) {
		return false
	}
	changed := false
	for i, score := range boosted {
		if i < len(original) && score > original[i] {
			changed = true
			break
		}
	}
	if !changed {
		return false
	}
	baseline := labelScoredSpans(p, wins, original, blocked, n, rate, false)
	return !windowInsideMe(clip, baseline)
}
