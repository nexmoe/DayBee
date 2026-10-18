package voiceprint

import (
	"reflect"
	"strings"
)

const systemPoolCap = 500

// runtimeState is the in-memory system cohort. It never changes Calibration.
type runtimeState struct {
	ready   bool
	adapt   bool
	context EnrollmentContext
	system  [][]float32
}

type runtimePermit struct {
	ok       bool
	revision uint64
	context  EnrollmentContext
	system   [][]float32
}

// ResetRuntime drops the system pool and starts a new device/AEC generation.
func (m *Manager) ResetRuntime(context EnrollmentContext) {
	if m == nil {
		return
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	m.noteChangeLocked()
	m.runtime.ready = true
	m.runtime.context = context
}

// RuntimeCounts reports the saved adaptation pool and the in-memory system pool.
func (m *Manager) RuntimeCounts() (adaptation, system int) {
	if m == nil {
		return 0, 0
	}
	m.profileMu.RLock()
	defer m.profileMu.RUnlock()
	if m.profile != nil {
		adaptation = len(m.profile.Adaptation)
	}
	return adaptation, len(m.runtime.system)
}

// MatchSpansContext labels audio and may record system-reference vectors.
// A profile that does not match this device or echo setting stays uncertain.
// A stale snapshot is still labeled when it is compatible with the context
// passed for this call, but it cannot update the current pool. MatchSpans
// itself has no context and never learns.
func (m *Manager) MatchSpansContext(p *Profile, samples []float32, rate int, ref *Reference, context EnrollmentContext, relax bool) (spans []IdentitySpan) {
	n := len(samples)
	defer func() {
		if recover() != nil {
			spans = uncertainAudio(n)
		}
	}()
	if n == 0 || m == nil || p == nil || !Compatible(p, context) {
		return uncertainAudio(n)
	}
	permit := m.takePermit(p, context)
	wins, err := m.Score(p, samples, rate)
	if err != nil || len(wins) == 0 {
		return uncertainAudio(n)
	}
	scores := DecisionScores(p, wins)
	echo, observed := observeSystem(m, samples, ref, wins, rate)
	echo = vetoWithPool(echo, wins, permit.system)
	labelScores := m.boostedScores(permit, p, relax, wins, scores, echo)
	spans = labelScoredSpans(p, wins, labelScores, echo, n, rate, relax)
	assisted := !relax && adaptationAssisted(p, wins, scores, labelScores, echo, spans, n, rate)
	pinOriginalAnchors(p, wins, scores, echo, spans, n)
	m.admitSystem(permit, p, context, filterSystemVectors(p, observed))
	m.learnAdaptation(permit, p, context, relax, ref, wins, scores, echo, n, rate, assisted)
	return spans
}

func (m *Manager) publishLocked(p *Profile) {
	m.revision++
	m.runtime.system = nil
	if p != nil {
		p.Revision = m.revision
	}
	m.profile = p
}

func (m *Manager) noteChangeLocked() {
	m.revision++
	m.runtime.system = nil
	if m.profile != nil {
		m.profile.Revision = m.revision
	}
}

func (m *Manager) takePermit(p *Profile, context EnrollmentContext) runtimePermit {
	if m == nil {
		return runtimePermit{}
	}
	m.profileMu.RLock()
	defer m.profileMu.RUnlock()
	if m.closed.Load() || !m.runtime.ready || !sameContext(m.runtime.context, context) || !sameEnrollment(m.profile, p) || !Compatible(p, context) {
		return runtimePermit{}
	}
	return runtimePermit{ok: true, revision: m.revision, context: context, system: copySystem(m.runtime.system)}
}

func (m *Manager) admitSystem(permit runtimePermit, p *Profile, context EnrollmentContext, vectors [][]float32) {
	vectors = filterSystemVectors(p, vectors)
	if m == nil || !permit.ok || len(vectors) == 0 {
		return
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.closed.Load() || m.revision != permit.revision || !m.runtime.ready || !sameContext(m.runtime.context, context) || !sameEnrollment(m.profile, p) || !Compatible(p, context) {
		return
	}
	m.runtime.system = pushSystem(m.runtime.system, vectors)
}

func copySystem(pool [][]float32) [][]float32 {
	out := make([][]float32, len(pool))
	for i, vector := range pool {
		out[i] = append([]float32(nil), vector...)
	}
	return out
}

func sameContext(a, b EnrollmentContext) bool {
	return a.EchoCancellation == b.EchoCancellation && trimDevice(a.Device) == trimDevice(b.Device)
}

func trimDevice(id string) string {
	return strings.TrimSpace(id)
}

func sameEnrollment(current, snap *Profile) bool {
	if current == nil || snap == nil || current.Revision == 0 || current.Revision != snap.Revision {
		return false
	}
	if current.Version != snap.Version || current.Model != snap.Model || current.Device != snap.Device || current.EchoCancellation != snap.EchoCancellation {
		return false
	}
	return sameVectors(current.Embeddings, snap.Embeddings) && sameCalibration(current.Calibration, snap.Calibration)
}

func sameVectors(a, b [][]float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

func sameCalibration(a, b *Calibration) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.TLow == b.TLow && a.THigh == b.THigh && a.Model == b.Model && a.CohortID == b.CohortID
}

func filterSystemVectors(p *Profile, vectors [][]float32) [][]float32 {
	floor := Threshold(ModelName)
	out := make([][]float32, 0, len(vectors))
	for _, vector := range vectors {
		if !validEmbedding(vector) {
			continue
		}
		if p != nil && ProfileScore(p, vector) >= floor {
			continue
		}
		out = append(out, append([]float32(nil), vector...))
	}
	return out
}

func pushSystem(pool [][]float32, extra [][]float32) [][]float32 {
	pool = append(pool, extra...)
	if len(pool) <= systemPoolCap {
		return pool
	}
	return append([][]float32(nil), pool[len(pool)-systemPoolCap:]...)
}

// vetoWithPool can only add rejections. A low system similarity never clears one.
func vetoWithPool(echo []bool, wins []WindowScore, pool [][]float32) []bool {
	out := make([]bool, len(wins))
	for i := range out {
		if echo != nil && i < len(echo) && echo[i] {
			out[i] = true
		}
	}
	floor := Threshold(ModelName)
	for i, win := range wins {
		if out[i] {
			continue
		}
		for _, vector := range pool {
			sim := Cosine(win.Embedding, vector)
			if sim >= floor && sim >= win.Score-echoMargin {
				out[i] = true
				break
			}
		}
	}
	return out
}

func observeSystem(m *Manager, mic []float32, ref *Reference, wins []WindowScore, rate int) ([]bool, [][]float32) {
	if ref == nil || EchoWindows == nil {
		return nil, nil
	}
	if defaultEcho() {
		return echoAnalysis(m, mic, ref, wins, rate)
	}
	return EchoWindows(m, mic, ref, wins, rate), nil
}

func defaultEcho() bool {
	return EchoWindows != nil && reflect.ValueOf(EchoWindows).Pointer() == reflect.ValueOf(echoWindows).Pointer()
}
