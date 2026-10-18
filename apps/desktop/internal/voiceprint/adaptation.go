package voiceprint

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
)

const (
	adaptationCap   = 32
	adaptationBoost = 0.3
	adaptationRaw   = float32(0.8)
	adaptationFloor = float32(3)
	adaptationRun   = 3
)

// SetAdaptationEnabled turns learning and score boosts on or off.
// The default is off. A change invalidates learning permits already taken.
func (m *Manager) SetAdaptationEnabled(enabled bool) {
	if m == nil {
		return
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.runtime.adapt == enabled {
		return
	}
	m.runtime.adapt = enabled
	m.bumpRevisionLocked()
}

// ClearAdaptation removes the saved adaptation pool and progress counters.
// The original enrollment stays. A failed write leaves that profile unchanged.
func (m *Manager) ClearAdaptation() error {
	if m == nil {
		return errors.New("Voiceprint manager is unavailable.")
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.closed.Load() {
		return errors.New("Voiceprint manager is closed.")
	}
	if m.profile == nil {
		return errors.New("A valid speaker embedding is required.")
	}
	next := clone(m.profile)
	next.Adaptation = nil
	next.AdaptationProgress = &AdaptationProgress{}
	if err := writeProfileFile(m.path, next); err != nil {
		return err
	}
	m.bumpRevisionLocked()
	next.Revision = m.revision
	m.profile = next
	return nil
}

func (m *Manager) bumpRevisionLocked() {
	m.revision++
	if m.profile != nil {
		m.profile.Revision = m.revision
	}
}

func (m *Manager) boostedScores(permit runtimePermit, p *Profile, relax bool, wins []WindowScore, scores []float32, blocked []bool) []float32 {
	if m == nil || relax || !permit.ok || !calibrationOK(p) {
		return scores
	}
	m.profileMu.RLock()
	current := m.runtime.adapt && m.revision == permit.revision && m.profile != nil
	m.profileMu.RUnlock()
	if !current || len(p.Adaptation) == 0 {
		return scores
	}
	return applyBoost(p, p.Adaptation, wins, scores, blocked)
}

func (m *Manager) learnAdaptation(permit runtimePermit, p *Profile, context EnrollmentContext, relax bool, ref *Reference, wins []WindowScore, scores []float32, blocked []bool, n, rate int, assisted bool) {
	if m == nil || relax || !permit.ok || !calibrationOK(p) {
		return
	}
	var vectors [][]float32
	if confirmedSilence(ref) {
		vectors = adaptationCandidates(p, wins, scores, blocked, n, rate)
	}
	m.storeAdaptationProgress(permit, p, context, vectors, assisted)
}

func (m *Manager) storeAdaptation(permit runtimePermit, p *Profile, context EnrollmentContext, vectors [][]float32) {
	m.storeAdaptationProgress(permit, p, context, vectors, false)
}

func (m *Manager) storeAdaptationProgress(permit runtimePermit, p *Profile, context EnrollmentContext, vectors [][]float32, assisted bool) {
	valid := make([][]float32, 0, len(vectors))
	for _, vector := range vectors {
		if validEmbedding(vector) {
			valid = append(valid, vector)
		}
	}
	if m == nil || !permit.ok || (len(valid) == 0 && !assisted) {
		return
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.closed.Load() || !m.runtime.adapt || m.revision != permit.revision || !m.runtime.ready || !sameContext(m.runtime.context, context) || !sameEnrollment(m.profile, p) || !Compatible(p, context) || !calibrationOK(m.profile) {
		return
	}
	next := clone(m.profile)
	next.Adaptation = pushAdaptation(next.Adaptation, valid)
	if next.AdaptationProgress == nil {
		next.AdaptationProgress = &AdaptationProgress{}
	}
	if len(valid) > 0 {
		next.AdaptationProgress.LearnedClips++
	}
	if assisted {
		next.AdaptationProgress.AssistedClips++
	}
	if err := writeProfileFile(m.path, next); err != nil {
		return
	}
	next.Revision = m.revision
	m.profile = next
}

func writeProfileFile(path string, profile *Profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return config.WriteJSON(path, profile)
}

func confirmedSilence(ref *Reference) bool {
	if ref == nil || !ref.Complete || len(ref.Samples) == 0 {
		return false
	}
	for _, sample := range ref.Samples {
		if !finite32(sample) {
			return false
		}
	}
	return rmsDBFS(ref.Samples) <= echoSilenceDB
}

func adaptationCandidates(p *Profile, wins []WindowScore, scores []float32, blocked []bool, n, rate int) [][]float32 {
	if !calibrationOK(p) || rate <= 0 || len(wins) == 0 {
		return nil
	}
	_, high := DecisionThresholds(p)
	need := high + 1
	if need < adaptationFloor {
		need = adaptationFloor
	}
	bounds := spanBounds(wins, n)
	proved := labelScoredSpans(p, wins, scores, blocked, n, rate, false)
	ok := make([]bool, len(wins))
	for i, win := range wins {
		if i >= len(scores) || blockedAt(blocked, i) || !finite32(win.Score) || !finite32(scores[i]) || win.Score < adaptationRaw || scores[i] < need || !validEmbedding(win.Embedding) {
			continue
		}
		ok[i] = true
	}
	var out [][]float32
	for i := 0; i < len(wins); {
		if !ok[i] {
			i++
			continue
		}
		j := i + 1
		for j < len(wins) && ok[j] {
			j++
		}
		if j < len(bounds) && bounds[j]-bounds[i] >= adaptationRun*rate {
			runStart, runEnd := bounds[i], bounds[j]
			for k := i; k < j; k++ {
				if wins[k].Start < runStart || wins[k].End > runEnd || !windowInsideMe(wins[k], proved) {
					continue
				}
				out = append(out, append([]float32(nil), wins[k].Embedding...))
			}
		}
		i = j
	}
	return out
}

// windowInsideMe reports whether the whole support lies in unboosted production me.
// A guard uncertain interval is not me, so that window cannot be learned.
func windowInsideMe(win WindowScore, spans []IdentitySpan) bool {
	if win.End <= win.Start {
		return false
	}
	cursor := win.Start
	for _, span := range spans {
		if span.End <= cursor || span.Start >= win.End {
			continue
		}
		if span.Start > cursor || span.Identity != spanMe {
			return false
		}
		cursor = span.End
		if cursor >= win.End {
			return true
		}
	}
	return false
}

func applyBoost(p *Profile, pool [][]float32, wins []WindowScore, scores []float32, blocked []bool) []float32 {
	out := append([]float32(nil), scores...)
	cohort, ok := loadedCohortFor(p)
	if !ok {
		return out
	}
	_, enrollStd, enrollOK := normTop(cohortProfileScores(p, cohort))
	if !enrollOK || !(enrollStd > 0) {
		return out
	}
	low, _ := DecisionThresholds(p)
	floor := Threshold(ModelName)
	adapted := &Profile{Embeddings: pool}
	for i, win := range wins {
		if i >= len(out) || blockedAt(blocked, i) || !finite32(win.Score) || !finite32(out[i]) || win.Score < floor || out[i] < low {
			continue
		}
		_, testStd, testOK := normTop(cohortCosines(win.Embedding, cohort))
		if !testOK || !(testStd > 0) {
			continue
		}
		delta := 0.5 * (1/enrollStd + 1/testStd) * float64(ProfileScore(adapted, win.Embedding)-win.Score)
		if delta > adaptationBoost {
			delta = adaptationBoost
		}
		if delta > 0 {
			boosted := float64(out[i]) + delta
			if boosted > float64(asNormMax) {
				boosted = float64(asNormMax)
			}
			out[i] = float32(boosted)
		}
	}
	return out
}

func pinOriginalAnchors(p *Profile, wins []WindowScore, scores []float32, blocked []bool, spans []IdentitySpan, n int) {
	bounds := spanBounds(wins, n)
	for i, span := range spans {
		spans[i].Anchor = false
		if span.Identity != spanMe {
			continue
		}
		owned, flags := scoresForSpan(bounds, scores, blocked, span)
		confirmed, _ := DecideProfile(p, owned, flags)
		spans[i].Anchor = confirmed == spanMe
	}
}

func pushAdaptation(pool, extra [][]float32) [][]float32 {
	for _, vector := range extra {
		if !validEmbedding(vector) {
			continue
		}
		pool = append(pool, append([]float32(nil), vector...))
	}
	if len(pool) <= adaptationCap {
		return pool
	}
	return append([][]float32(nil), pool[len(pool)-adaptationCap:]...)
}

func adaptationOK(vectors [][]float32) bool {
	if len(vectors) > adaptationCap {
		return false
	}
	for _, vector := range vectors {
		if !validEmbedding(vector) {
			return false
		}
	}
	return true
}

func blockedAt(blocked []bool, i int) bool {
	return blocked != nil && i < len(blocked) && blocked[i]
}
