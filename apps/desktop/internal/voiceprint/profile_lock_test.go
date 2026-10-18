package voiceprint

import (
	"sync"
	"testing"
	"time"
)

func TestSnapshotDoesNotWaitForInferenceLock(t *testing.T) {
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(testVectors()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	done := make(chan *Profile, 1)
	go func() {
		done <- m.Snapshot()
	}()
	select {
	case snap := <-done:
		if snap == nil || len(snap.Embeddings) != 1 || snap.Embeddings[0][0] != 1 {
			t.Fatalf("snapshot %+v", snap)
		}
	case <-time.After(time.Second):
		t.Fatal("Snapshot blocked while inference held m.mu")
	}
	m.mu.Unlock()
}

func TestCloseAndSaveDoNotRace(t *testing.T) {
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(testVectors()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = m.Save(testVectors())
	}()
	go func() {
		defer wg.Done()
		m.Close()
	}()
	wg.Wait()
	if err := m.Save(testVectors()); err == nil {
		t.Fatal("closed manager accepted a profile save")
	}
	snap := m.Snapshot()
	if snap != nil && (len(snap.Embeddings) != 1 || snap.Embeddings[0][0] != 1) {
		t.Fatalf("snapshot after close %+v", snap)
	}
}
