package voiceprint

import (
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAdaptationDefaultsOffAndDisableDoesNotScoreOrLearn(t *testing.T) {
	m := enrolledV3(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("adaptation started enabled")
	}
	m.ResetRuntime(ctx)
	current := m.Snapshot()
	wins, scores := confidentRun(0.9, 4)
	silent := &Reference{Samples: make([]float32, 16), Complete: true}
	m.learnAdaptation(m.takePermit(current, ctx), current, ctx, false, silent, wins, scores, nil, 48000, 16000, false)
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("disabled adaptation stored a vector")
	}
	m.SetAdaptationEnabled(true)
	permit := m.takePermit(m.Snapshot(), ctx)
	m.SetAdaptationEnabled(false)
	m.learnAdaptation(permit, current, ctx, false, silent, wins, scores, nil, 48000, 16000, false)
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("a permit from before disable stored a vector")
	}
	base := []float32{4}
	if got := m.boostedScores(permit, current, false, wins[:1], base, nil); len(got) != 1 || got[0] != 4 {
		t.Fatalf("disabled boost changed %v", got)
	}
}

func TestAdaptationPoolKeepsThirtyTwoNewest(t *testing.T) {
	m, ctx, permit, snap := learningPermit(t)
	for i := 1; i <= adaptationCap+1; i++ {
		vector := make([]float32, embeddingDimension)
		vector[0] = float32(i)
		vector[1] = 1
		m.storeAdaptation(permit, snap, ctx, [][]float32{vector})
	}
	if adapt, _ := m.RuntimeCounts(); adapt != adaptationCap {
		t.Fatalf("adaptation %d", adapt)
	}
	got := m.Snapshot().Adaptation
	if got[0][0] != 2 || got[len(got)-1][0] != float32(adaptationCap+1) {
		t.Fatalf("fifo %v .. %v", got[0][0], got[len(got)-1][0])
	}
	if m.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("learning replaced an enrollment vector")
	}
}

func TestAdaptationRoundTripAndClearKeepsEnrollment(t *testing.T) {
	m, ctx, permit, snap := learningPermit(t)
	vector := make([]float32, embeddingDimension)
	vector[1] = 1
	m.storeAdaptation(permit, snap, ctx, [][]float32{vector})
	root := filepath.Dir(filepath.Dir(m.path))
	reloaded, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded := reloaded.Snapshot()
	if len(loaded.Adaptation) != 1 || loaded.Adaptation[0][1] != 1 || loaded.Embeddings[0][0] != 1 {
		t.Fatalf("reloaded %+v", loaded)
	}
	loaded.Adaptation[0][1] = 9
	if m.Snapshot().Adaptation[0][1] == 9 {
		t.Fatal("snapshot shares adaptation memory")
	}
	stale := m.takePermit(m.Snapshot(), ctx)
	if err := m.ClearAdaptation(); err != nil {
		t.Fatal(err)
	}
	cleared := m.Snapshot()
	if len(cleared.Adaptation) != 0 || cleared.Embeddings[0][0] != 1 || cleared.Revision == stale.revision {
		t.Fatalf("clear %+v", cleared)
	}
	m.storeAdaptation(stale, snap, ctx, [][]float32{vector})
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("old permit refilled the cleared pool")
	}
}

func TestAdaptationRejectsUnprovenReferencesAndContexts(t *testing.T) {
	m, ctx, permit, snap := learningPermit(t)
	wins, scores := confidentRun(0.9, 4)
	vector := [][]float32{wins[0].Embedding}
	if got := adaptationCandidates(snap, wins, scores, nil, 48000, 16000); len(got) != len(wins) {
		t.Fatalf("proven run stored %d", len(got))
	}
	weak, weakScores := confidentRun(0.79, 4)
	if len(adaptationCandidates(snap, weak, weakScores, nil, 48000, 16000)) != 0 {
		t.Fatal("raw below 0.8 was learned")
	}
	lowZ, lowZScores := confidentRun(0.9, 2.9)
	if len(adaptationCandidates(snap, lowZ, lowZScores, nil, 48000, 16000)) != 0 {
		t.Fatal("z below the fixed floor was learned from a boost-sized gap")
	}
	blocked, blockedScores := confidentRun(0.9, 4)
	echo := make([]bool, len(blocked))
	echo[1] = true
	if len(adaptationCandidates(snap, blocked, blockedScores, echo, 48000, 16000)) != 0 {
		t.Fatal("a vetoed window still joined a 3 second run")
	}
	silent := &Reference{Samples: []float32{0, 0}, Complete: true}
	loud := &Reference{Samples: []float32{0.2, -0.2}, Complete: true}
	partial := &Reference{Samples: []float32{0, 0}, Complete: false}
	broken := &Reference{Samples: []float32{float32(math.NaN())}, Complete: true}
	for _, ref := range []*Reference{nil, loud, partial, broken} {
		m.learnAdaptation(permit, snap, ctx, false, ref, wins, scores, nil, 48000, 16000, false)
	}
	m.learnAdaptation(permit, snap, ctx, true, silent, wins, scores, nil, 48000, 16000, false)
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("incomplete, loud, relaxed, or missing audio was learned")
	}
	m.ResetRuntime(EnrollmentContext{Device: "native:mic-b"})
	m.storeAdaptation(permit, snap, ctx, vector)
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("a device change accepted the old learning permit")
	}
}

func TestAdaptationBoostCannotBypassOriginalProof(t *testing.T) {
	m := enrolledV3(t)
	profile := m.Snapshot()
	window := make([]float32, embeddingDimension)
	window[0], window[1] = 1, 1
	wins := []WindowScore{{Start: 0, End: 16000, Score: ProfileScore(profile, window), Embedding: window}}
	if wins[0].Score < Threshold(ModelName) {
		t.Fatalf("fixture raw %v", wins[0].Score)
	}
	pool := [][]float32{window}
	raised := applyBoost(profile, pool, wins, []float32{4}, nil)
	if len(raised) != 1 || !(raised[0] > 4) || raised[0] > 4+float32(adaptationBoost)+1e-4 {
		t.Fatalf("boost %v", raised)
	}
	if got := applyBoost(profile, pool, wins, []float32{4}, []bool{true}); got[0] != 4 {
		t.Fatalf("echo was boosted: %v", got)
	}
	lowRaw := []WindowScore{{Score: 0.4, Embedding: window}}
	if got := applyBoost(profile, pool, lowRaw, []float32{4}, nil); got[0] != 4 {
		t.Fatalf("raw below the floor was boosted: %v", got)
	}
	if got := applyBoost(profile, pool, wins, []float32{0.1}, nil); got[0] != 0.1 {
		t.Fatalf("z below low was boosted: %v", got)
	}
	if got := applyBoost(profile, pool, wins, []float32{asNormMax}, nil); got[0] != asNormMax {
		t.Fatalf("saturated z became %v", got[0])
	}
	nanRaw := []WindowScore{{Score: float32(math.NaN()), Embedding: window}}
	if got := applyBoost(profile, pool, nanRaw, []float32{4}, nil); got[0] != 4 {
		t.Fatalf("non-finite raw was boosted: %v", got)
	}
	if got := applyBoost(profile, pool, wins, []float32{float32(math.NaN())}, nil); len(got) != 1 || !math.IsNaN(float64(got[0])) {
		t.Fatalf("non-finite z was boosted: %v", got)
	}
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.SetAdaptationEnabled(true)
	m.ResetRuntime(ctx)
	frozenProfile := m.Snapshot()
	permit := m.takePermit(frozenProfile, ctx)
	m.storeAdaptation(permit, frozenProfile, ctx, pool)
	if got := m.boostedScores(permit, frozenProfile, false, wins, []float32{4}, nil); len(got) != 1 || got[0] != 4 {
		t.Fatalf("snapshot without a pool was boosted by a later pool: %v", got)
	}
	live := m.Snapshot()
	if got := m.boostedScores(permit, live, false, wins, []float32{4}, nil); len(got) != 1 || !(got[0] > 4) {
		t.Fatalf("the snapshot that contains the pool did not boost: %v", got)
	}
	if err := m.ClearAdaptation(); err != nil {
		t.Fatal(err)
	}
	if got := m.boostedScores(permit, live, false, wins, []float32{4}, nil); got[0] != 4 {
		t.Fatalf("cleared generation still boosted: %v", got)
	}
	legacy := &Profile{Version: 2, Model: ModelName, Embeddings: testVectors(), Revision: 1}
	if got := applyBoost(legacy, pool, wins, []float32{4}, nil); got[0] != 4 {
		t.Fatal("legacy profile was boosted")
	}
	boosted := labelScoredSpans(profile, []WindowScore{{Start: 0, End: 16000}}, []float32{0.9}, nil, 16000, 16000, false)
	pinOriginalAnchors(profile, []WindowScore{{Start: 0, End: 16000, Score: 0.4}}, []float32{0.4}, nil, boosted, 16000)
	if len(boosted) != 1 || boosted[0].Identity != "me" || boosted[0].Anchor {
		t.Fatalf("boosted me kept an original anchor: %+v", boosted)
	}
}

func TestIncompatibleContextDoesNotLabelOrLearn(t *testing.T) {
	m := enrolledV3(t)
	m.SetAdaptationEnabled(true)
	snap := m.Snapshot()
	samples := make([]float32, 32000)
	ref := &Reference{Samples: make([]float32, 32), Complete: true}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ctx := range []EnrollmentContext{
		{Device: "native:mic-b"},
		{Device: "native:mic-a", EchoCancellation: true},
	} {
		done := make(chan []IdentitySpan, 1)
		go func(ctx EnrollmentContext) {
			done <- m.MatchSpansContext(snap, samples, voiceSampleRate, ref, ctx, false)
		}(ctx)
		select {
		case spans := <-done:
			if len(spans) != 1 || spans[0].Start != 0 || spans[0].End != len(samples) || spans[0].Identity != spanUncertain {
				t.Fatalf("context %+v labeled %+v", ctx, spans)
			}
		case <-time.After(time.Second):
			t.Fatalf("context %+v waited for inference", ctx)
		}
	}
	if adapt, system := m.RuntimeCounts(); adapt != 0 || system != 0 {
		t.Fatalf("incompatible context updated runtime %d %d", adapt, system)
	}
}

func TestStaleCompatibleSnapshotIsLabeledWithoutLearning(t *testing.T) {
	m := enrolledV3(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.SetAdaptationEnabled(true)
	m.ResetRuntime(ctx)
	snap := m.Snapshot()
	m.ResetRuntime(EnrollmentContext{Device: "native:mic-b"})
	m.mu.Lock()
	done := make(chan []IdentitySpan, 1)
	go func() {
		done <- m.MatchSpansContext(snap, make([]float32, 32000), voiceSampleRate, &Reference{Samples: []float32{0}, Complete: true}, ctx, false)
	}()
	select {
	case spans := <-done:
		t.Fatalf("compatible stale snapshot skipped analysis: %+v", spans)
	case <-time.After(50 * time.Millisecond):
	}
	m.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("analysis did not finish after inference was released")
	}
	if adapt, _ := m.RuntimeCounts(); adapt != 0 {
		t.Fatal("stale snapshot learned")
	}
	if spans := m.MatchSpans(snap, make([]float32, 1000), voiceSampleRate, nil); len(spans) != 1 || spans[0].Identity != spanUncertain {
		t.Fatalf("context-free MatchSpans changed: %+v", spans)
	}
}

func TestAdaptationWindowMustFitInsideTheRun(t *testing.T) {
	profile := enrolledV3(t).Snapshot()
	wins := make([]WindowScore, 6)
	scores := make([]float32, len(wins))
	for i := range wins {
		wins[i] = WindowScore{Start: i * 8000, End: i*8000 + 24000, Score: 0.9, Embedding: testVectors()[0]}
		scores[i] = 4
	}
	got := adaptationCandidates(profile, wins, scores, nil, 48000, 16000)
	if len(got) != 4 {
		t.Fatalf("windows crossing the run were kept: %d", len(got))
	}
	scores[5] = float32(math.Inf(1))
	if len(adaptationCandidates(profile, wins, scores, nil, 48000, 16000)) != 0 {
		t.Fatal("infinite z remained inside the run")
	}
	scores[5] = 4
	wins[0].Score = float32(math.NaN())
	if len(adaptationCandidates(profile, wins, scores, nil, 48000, 16000)) != 0 {
		t.Fatal("non-finite raw remained inside the run")
	}
}

func TestAdaptationSaveFailureKeepsTheProfile(t *testing.T) {
	m, ctx, permit, snap := learningPermit(t)
	vector := make([]float32, embeddingDimension)
	vector[1] = 1
	m.storeAdaptation(permit, snap, ctx, [][]float32{vector})
	blockProfileStorage(t, m)
	if err := m.ClearAdaptation(); err == nil {
		t.Fatal("unwritable profile was cleared")
	}
	if adapt, _ := m.RuntimeCounts(); adapt != 1 || m.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("failed clear changed the profile")
	}
}

func TestAdaptationClearToggleAndMatchDoNotRace(t *testing.T) {
	m := enrolledV3(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.SetAdaptationEnabled(true)
	m.ResetRuntime(ctx)
	snap := m.Snapshot()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = m.ClearAdaptation()
		}()
		go func(on bool) {
			defer wg.Done()
			m.SetAdaptationEnabled(on)
		}(i%2 == 0)
		go func() {
			defer wg.Done()
			m.MatchSpansContext(snap, make([]float32, 1000), voiceSampleRate, &Reference{Samples: []float32{0}, Complete: true}, ctx, false)
			_, _ = m.RuntimeCounts()
		}()
	}
	wg.Wait()
}

func learningPermit(t *testing.T) (*Manager, EnrollmentContext, runtimePermit, *Profile) {
	t.Helper()
	m := enrolledV3(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.SetAdaptationEnabled(true)
	m.ResetRuntime(ctx)
	snap := m.Snapshot()
	permit := m.takePermit(snap, ctx)
	if !permit.ok {
		t.Fatal("current calibrated profile was not permitted")
	}
	return m, ctx, permit, snap
}

func confidentRun(raw, z float32) ([]WindowScore, []float32) {
	wins := make([]WindowScore, 6)
	scores := make([]float32, len(wins))
	for i := range wins {
		end := i*8000 + windowSamples
		if end > 48000 {
			end = 48000
		}
		wins[i] = WindowScore{Start: i * 8000, End: end, Score: raw, Embedding: testVectors()[0]}
		scores[i] = z
	}
	return wins, scores
}
