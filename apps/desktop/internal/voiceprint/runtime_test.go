package voiceprint

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
)

func TestSystemPoolCapsAtFiveHundred(t *testing.T) {
	pool := [][]float32{}
	for i := 1; i <= systemPoolCap+1; i++ {
		vector := make([]float32, embeddingDimension)
		vector[0] = float32(i)
		pool = pushSystem(pool, [][]float32{vector})
	}
	if len(pool) != systemPoolCap {
		t.Fatalf("pool %d", len(pool))
	}
	if pool[0][0] != 2 || pool[len(pool)-1][0] != float32(systemPoolCap+1) {
		t.Fatalf("fifo lost the wrong end: %v .. %v", pool[0][0], pool[len(pool)-1][0])
	}
}

func TestSystemPoolRejectsIllegalAndSelfVectors(t *testing.T) {
	profile := &Profile{Version: 2, Model: ModelName, Embeddings: testVectors()}
	self := append([]float32(nil), testVectors()[0]...)
	other := make([]float32, embeddingDimension)
	other[2] = 1
	nan := append([]float32(nil), other...)
	nan[0] = float32(math.NaN())
	got := filterSystemVectors(profile, [][]float32{nil, {1}, nan, make([]float32, embeddingDimension), self, other})
	if len(got) != 1 || got[0][2] != 1 {
		t.Fatalf("kept %d vectors", len(got))
	}
	if ProfileScore(profile, self) < Threshold(ModelName) {
		t.Fatal("self fixture is below the raw floor")
	}
}

func TestSystemVetoCannotRaiseAcceptance(t *testing.T) {
	profile := &Profile{Version: 2, Model: ModelName, Embeddings: testVectors(), Revision: 1}
	me := testVectors()[0]
	other := make([]float32, embeddingDimension)
	other[2] = 1
	windows := []WindowScore{{Start: 0, End: 16000, Score: 0.9, Embedding: me}}
	open := labelScoredSpans(profile, windows, []float32{0.9}, []bool{false}, 16000, 16000, false)
	if len(open) != 1 || open[0].Identity != "me" {
		t.Fatalf("open label %+v", open)
	}
	veto := vetoWithPool([]bool{false}, windows, [][]float32{me})
	closed := labelScoredSpans(profile, windows, []float32{0.9}, veto, 16000, 16000, false)
	if len(closed) != 1 || closed[0].Identity == "me" {
		t.Fatalf("veto left me in place: %+v", closed)
	}
	low := []WindowScore{{Start: 0, End: 16000, Score: 0.2, Embedding: other}}
	before := labelScoredSpans(profile, low, []float32{0.2}, []bool{false}, 16000, 16000, false)
	after := labelScoredSpans(profile, low, []float32{0.2}, vetoWithPool([]bool{false}, low, [][]float32{me}), 16000, 16000, false)
	if before[0].Identity == "me" || after[0].Identity == "me" || !vetoWithPool([]bool{true}, low, nil)[0] {
		t.Fatalf("veto changed a rejection into me: %+v %+v", before, after)
	}
}

func TestRevisionAndDeviceBlockStaleSystemWrites(t *testing.T) {
	m := enrolledManager(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.ResetRuntime(ctx)
	current := m.Snapshot()
	if current.Revision == 0 {
		t.Fatal("snapshot dropped the revision")
	}
	other := make([]float32, embeddingDimension)
	other[2] = 1
	permit := m.takePermit(current, ctx)
	m.admitSystem(permit, current, ctx, [][]float32{other})
	if _, n := m.RuntimeCounts(); n != 1 {
		t.Fatalf("current snapshot did not record the system vector: %d", n)
	}
	raw, _ := json.Marshal(current)
	if json.Unmarshal(raw, &Profile{}) != nil {
		t.Fatal(raw)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["revision"]; ok {
		t.Fatal("revision was serialized")
	}
	lookalike := clone(current)
	lookalike.Embeddings[0][0] = 0
	lookalike.Embeddings[0][1] = 1
	if m.takePermit(lookalike, ctx).ok {
		t.Fatal("model name alone matched a different enrollment")
	}
	m.ResetRuntime(EnrollmentContext{Device: "native:mic-b", EchoCancellation: true})
	m.admitSystem(permit, current, ctx, [][]float32{other})
	if _, n := m.RuntimeCounts(); n != 0 {
		t.Fatalf("device change kept or refilled the pool: %d", n)
	}
	again := m.Snapshot()
	if again.Revision == current.Revision {
		t.Fatal("reset reused the revision")
	}
	rolled := append([][]float32(nil), again.Embeddings...)
	if err := m.Save(rolled); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Revision == again.Revision {
		t.Fatal("rollback save reused the revision")
	}
	m.admitSystem(m.takePermit(again, EnrollmentContext{Device: "native:mic-b", EchoCancellation: true}), again, EnrollmentContext{Device: "native:mic-b", EchoCancellation: true}, [][]float32{other})
	if _, n := m.RuntimeCounts(); n != 0 {
		t.Fatal("pre-save snapshot wrote into the new profile")
	}
}

func TestIncompatiblePermitKeepsTheFrozenPool(t *testing.T) {
	m := enrolledV3(t)
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.ResetRuntime(ctx)
	current := m.Snapshot()
	other := make([]float32, embeddingDimension)
	other[2] = 1
	permit := m.takePermit(current, ctx)
	m.admitSystem(permit, current, ctx, [][]float32{other})
	frozen := m.takePermit(current, ctx)
	if !frozen.ok || len(frozen.system) != 1 || frozen.system[0][2] != 1 {
		t.Fatalf("frozen pool %+v", frozen)
	}
	wrong := EnrollmentContext{Device: "native:mic-b"}
	denied := m.takePermit(current, wrong)
	if denied.ok || len(denied.system) != 0 {
		t.Fatal("incompatible device received the system pool")
	}
	external := clone(current)
	external.Revision++
	if stale := m.takePermit(external, ctx); stale.ok || len(stale.system) != 0 {
		t.Fatal("external snapshot received the system pool")
	}
	window := []WindowScore{{Score: 0.9, Embedding: other}}
	if vetoWithPool(nil, window, denied.system)[0] || vetoWithPool(nil, window, frozen.system)[0] == false {
		t.Fatal("veto used the wrong pool")
	}
	m.ResetRuntime(ctx)
	newer := m.Snapshot()
	fresh := make([]float32, embeddingDimension)
	fresh[3] = 1
	m.admitSystem(m.takePermit(newer, ctx), newer, ctx, [][]float32{fresh})
	if len(frozen.system) != 1 || frozen.system[0][2] != 1 || frozen.system[0][3] != 0 {
		t.Fatal("reset mixed a new vector into the frozen pool")
	}
	if _, n := m.RuntimeCounts(); n != 1 {
		t.Fatalf("live pool %d", n)
	}
}

func TestCustomEchoHookIsUsedAsIs(t *testing.T) {
	previous := EchoWindows
	EchoWindows = func(m *Manager, mic []float32, ref *Reference, wins []WindowScore, rate int) []bool {
		return []bool{true, false}
	}
	defer func() { EchoWindows = previous }()
	flags, vectors := observeSystem(&Manager{}, []float32{1}, &Reference{Samples: []float32{1}}, []WindowScore{{}, {}}, voiceSampleRate)
	if vectors != nil || len(flags) != 2 || !flags[0] || flags[1] {
		t.Fatalf("custom echo flags %v vectors %v", flags, vectors)
	}
}

func TestMatchWithoutContextDoesNotLearn(t *testing.T) {
	m := enrolledManager(t)
	snap := m.Snapshot()
	ref := &Reference{Samples: make([]float32, 16000)}
	m.MatchSpans(snap, make([]float32, 16000), voiceSampleRate, ref)
	m.MatchSpansRelaxed(snap, make([]float32, 16000), voiceSampleRate, ref, true)
	if adaptation, system := m.RuntimeCounts(); adaptation != 0 || system != 0 {
		t.Fatalf("old match learned %d %d", adaptation, system)
	}
	ctx := EnrollmentContext{Device: "native:mic-a"}
	m.ResetRuntime(ctx)
	m.MatchSpans(m.Snapshot(), make([]float32, 32000), voiceSampleRate, ref)
	if _, system := m.RuntimeCounts(); system != 0 {
		t.Fatal("MatchSpans wrote the system pool")
	}
}

func TestRuntimeResetClearAndMatchDoNotRace(t *testing.T) {
	m := enrolledManager(t)
	snap := m.Snapshot()
	ctx := EnrollmentContext{Device: "native:mic-a"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			m.ResetRuntime(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = m.Clear()
		}()
		go func() {
			defer wg.Done()
			m.MatchSpansContext(snap, make([]float32, 1000), voiceSampleRate, &Reference{Samples: make([]float32, 1000)}, ctx, false)
			_, _ = m.RuntimeCounts()
		}()
	}
	wg.Wait()
	if adaptation, _ := m.RuntimeCounts(); adaptation != 0 {
		t.Fatal("adaptation pool was created")
	}
}

func enrolledV3(t *testing.T) *Manager {
	t.Helper()
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := &Profile{
		Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(),
		Device: "native:mic-a", EchoCancellation: false,
		Calibration: &Calibration{TLow: 0.2, THigh: 0.5, Model: ModelName, CohortID: meta.BlobSHA256},
	}
	if err := m.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	return m
}

func enrolledManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(testVectors()); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Revision == 0 {
		t.Fatal("new enrollment has no revision")
	}
	return m
}
