package voiceprint

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testVectors() [][]float32 {
	vector := make([]float32, embeddingDimension)
	vector[0] = 1
	return [][]float32{vector}
}

func TestProfilePersistenceIsolationAndClear(t *testing.T) {
	root := t.TempDir()
	m, err := New(root, nil)
	if err != nil || m.Enrolled() {
		t.Fatal("new profile must be optional", err)
	}
	vectors := testVectors()
	if err := m.Save(vectors); err != nil {
		t.Fatal(err)
	}
	vectors[0][0] = 2
	snapshot := m.Snapshot()
	snapshot.Embeddings[0][0] = 3
	if m.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("caller mutated the frozen profile")
	}
	reloaded, err := New(root, nil)
	if err != nil || !reloaded.Enrolled() || reloaded.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("profile did not survive restart", err)
	}
	info, err := os.Stat(filepath.Join(root, "voiceprint", "profile.json"))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("profile must be private", err)
	}
	if err := reloaded.Clear(); err != nil || reloaded.Enrolled() {
		t.Fatal("clear retained the profile", err)
	}
}

func TestInvalidSavePreservesPreviousProfile(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	if err := m.Save(testVectors()); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][][]float32{nil, {{1}}, {make([]float32, embeddingDimension)}} {
		if err := m.Save(invalid); err == nil {
			t.Fatal("accepted an invalid profile")
		}
	}
	nan := testVectors()
	nan[0][0] = float32(math.NaN())
	if err := m.Save(nan); err == nil || m.Snapshot().Embeddings[0][0] != 1 {
		t.Fatal("invalid save replaced the working profile")
	}
}

func vectors(n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = make([]float32, embeddingDimension)
		out[i][0] = 1
	}
	return out
}

func TestVersion1ProfileStillLoads(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voiceprint", "profile.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Profile{Version: 1, Model: ModelName, Embeddings: vectors(2), CreatedAt: "then"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(root, nil)
	if err != nil || !m.Enrolled() || m.Snapshot().Version != 1 || len(m.Snapshot().Embeddings) != 2 {
		t.Fatal("version 1 profile was not kept", err)
	}
	if err := m.Save(vectors(9)); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root, nil)
	if err != nil || reloaded.Snapshot().Version != 2 || len(reloaded.Snapshot().Embeddings) != 9 {
		t.Fatal("save did not write a version 2 profile", err)
	}
	if err := reloaded.Save(vectors(64)); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Save(vectors(65)); err == nil || len(reloaded.Snapshot().Embeddings) != 64 {
		t.Fatal("the 64-vector cap was not enforced")
	}
}

func TestCAMPlusProfileRequiresReenrollment(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "voiceprint", "profile.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(Profile{Version: version,
				Model: "3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx", Embeddings: testVectors()})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			m, err := New(root, nil)
			if err == nil || m == nil || m.Enrolled() {
				t.Fatal("old-model vectors must not be used by the new model")
			}
			defer m.Close()
			if retained, err := os.ReadFile(path); err != nil || string(retained) != string(raw) {
				t.Fatal("model migration changed the original profile before re-enrollment", err)
			}
			if err := m.Save(testVectors()); err != nil {
				t.Fatal(err)
			}
			reloaded, err := New(root, nil)
			if err != nil || !reloaded.Enrolled() {
				t.Fatal("new-model enrollment did not survive restart", err)
			}
			defer reloaded.Close()
			if p := reloaded.Snapshot(); p.Model != ModelName || p.Version != 2 {
				t.Fatal("re-enrollment did not replace the old model profile")
			}
		})
	}
}

func TestEchoCancellationFalseRoundTrip(t *testing.T) {
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := &Profile{
		Version: calibratedVersion, Model: ModelName, Embeddings: testVectors(),
		Device: "native:mic-1", EchoCancellation: false,
		Calibration: &Calibration{TLow: 0.2, THigh: 0.5, Model: ModelName, CohortID: meta.BlobSHA256},
	}
	if err := m.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "voiceprint", "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"echoCancellation": false`) {
		t.Fatalf("false echo flag omitted: %s", raw)
	}
	reloaded, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Snapshot(); got == nil || got.EchoCancellation || got.Version != calibratedVersion {
		t.Fatalf("reloaded %+v", got)
	}
}

func TestCorruptProfileRetainedAndUnavailableMatchIsEmpty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voiceprint", "profile.json")
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, []byte("broken"), 0600)
	m, err := New(root, func() string { return "/missing/model.onnx" })
	if m == nil || err == nil || m.Enrolled() {
		t.Fatal("corrupt profile should return a usable unregistered manager")
	}
	if raw, _ := os.ReadFile(path); string(raw) != "broken" {
		t.Fatal("corrupt source was deleted")
	}
	if identity, score := m.Match(&Profile{Embeddings: testVectors()}, make([]float32, 32000), 16000, nil); identity != "" || score != 0 {
		t.Fatal("an unavailable model assigned an identity")
	}
	m.Close()
	if _, err := m.Embed(make([]float32, 32000), 16000); err == nil {
		t.Fatal("closed manager accepted inference")
	}
}
