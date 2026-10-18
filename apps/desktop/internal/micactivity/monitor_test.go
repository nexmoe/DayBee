package micactivity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLifecycleExcludesSelfAndMergesAppProcesses(t *testing.T) {
	now := time.Date(2026, 10, 9, 23, 59, 58, 0, time.UTC)
	apps := []Application{{ID: "daybee", PID: 10}, {ID: "zoom", Name: "Zoom", Icon: "logo", PID: 20},
		{ID: "zoom", Name: "Zoom Helper", PID: 21}, {ID: "browser", Name: "Browser", PID: 30}}
	m, err := Open(t.TempDir(), func() ([]Application, error) { return apps, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(now)
	items := m.Snapshot().Items
	if len(items) != 2 || items[0].AppID != "zoom" || items[0].AppName != "Zoom" || items[0].AppIcon != "logo" || items[0].EndedAt != nil {
		t.Fatalf("unexpected active applications: %+v", items)
	}
	zoomID := items[0].ID
	apps = []Application{{ID: "zoom", Name: "Zoom", PID: 21}}
	m.Poll(now.Add(3 * time.Second)) // Midnight and helper handoff do not split use.
	items = m.Snapshot().Items
	if len(items) != 2 || items[0].ID != zoomID || items[0].EndedAt != nil || items[1].EndedAt == nil {
		t.Fatalf("unexpected handoff: %+v", items)
	}
	apps = nil
	m.Poll(now.Add(4 * time.Second))
	items = m.Snapshot().Items
	if *items[0].EndedAt != now.Add(4*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("wrong end time: %+v", items[0])
	}
	apps = []Application{{ID: "zoom", Name: "Zoom", PID: 22}}
	m.Poll(now.Add(5 * time.Second))
	if items := m.Snapshot().Items; len(items) != 3 || items[0].ID == zoomID || items[0].EndedAt != nil {
		t.Fatalf("restart must create a new session: %+v", items)
	}
	files, _ := filepath.Glob(filepath.Join(m.directory, "*.json"))
	if len(files) != 2 {
		t.Fatalf("expected two daily archives, got %v", files)
	}
}

func TestRecoveryUsesLastObservedTimeAndRetainsIcons(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	scan := func() ([]Application, error) {
		return []Application{{ID: "meet", Name: "Meet", Icon: "logo", PID: 20}}, nil
	}
	m, err := Open(directory, scan, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(now)
	m.Poll(now.Add(5 * time.Second))
	reopened, err := Open(directory, scan, 10) // Simulate crash, without Close.
	if err != nil {
		t.Fatal(err)
	}
	item := reopened.Snapshot().Items[0]
	if item.EndedAt == nil || *item.EndedAt != now.Add(5*time.Second).Format(time.RFC3339Nano) || item.AppIcon != "logo" {
		t.Fatalf("incorrect recovered session: %+v", item)
	}
	reopened.Poll(now.Add(time.Hour))
	if len(reopened.Snapshot().Items) != 2 {
		t.Fatal("application downtime must not be merged into a session")
	}
	if err := reopened.Close(now.Add(time.Hour + time.Second)); err != nil {
		t.Fatal(err)
	}
	reopened.Poll(now.Add(2 * time.Hour))
	if reopened.Snapshot().Items[0].EndedAt == nil {
		t.Fatal("closed monitor must not start new sessions")
	}
}

func TestScanFailureDoesNotEndUsageAndSuspendDoes(t *testing.T) {
	now := time.Now()
	var scanErr error
	scans := 0
	m, err := Open(t.TempDir(), func() ([]Application, error) {
		scans++
		return []Application{{ID: "meet", PID: 20}}, scanErr
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(now)
	scanErr = errors.New("audio service restarted")
	m.Poll(now.Add(time.Second))
	if snapshot := m.Snapshot(); snapshot.Error == nil || snapshot.Items[0].EndedAt != nil {
		t.Fatalf("failed scan was treated as a stop: %+v", snapshot)
	}
	if err := m.Suspend(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	m.Poll(now.Add(3 * time.Second))
	if scans != 2 || m.Snapshot().Items[0].EndedAt == nil {
		t.Fatal("suspend must end sessions and stop polling")
	}
	scanErr = nil
	m.Resume()
	m.Poll(now.Add(time.Hour))
	if snapshot := m.Snapshot(); snapshot.Error != nil || len(snapshot.Items) != 2 {
		t.Fatalf("resume did not create a new session: %+v", snapshot)
	}
}

func TestUnsupportedSystemAndCorruptHistory(t *testing.T) {
	directory := t.TempDir()
	m, err := Open(directory, func() ([]Application, error) { return nil, ErrUnsupported }, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(time.Now())
	if snapshot := m.Snapshot(); snapshot.Supported || snapshot.Error != nil || snapshot.Items == nil {
		t.Fatalf("unexpected unsupported snapshot: %+v", snapshot)
	}
	path := filepath.Join(directory, "20261009.json")
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory, NativeScan, 10); err == nil {
		t.Fatal("corrupt archive must not be silently overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{broken" {
		t.Fatal("corrupt archive was modified")
	}
}

func TestSaveFailureRetriesWithoutDroppingSession(t *testing.T) {
	directory := t.TempDir()
	blocker := filepath.Join(directory, "blocked")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(blocker, func() ([]Application, error) { return []Application{{ID: "meet", PID: 20}}, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Poll(now)
	if m.Snapshot().Error == nil {
		t.Fatal("save failure must be visible")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	m.Poll(now.Add(5 * time.Second))
	if snapshot := m.Snapshot(); snapshot.Error != nil || len(snapshot.Items) != 1 {
		t.Fatalf("save did not retry: %+v", snapshot)
	}
	data, err := os.ReadFile(m.dayPath(now.Format("20060102")))
	if err != nil || !json.Valid(data) {
		t.Fatalf("retry did not save a valid archive: %s %v", data, err)
	}
}

func TestNativeScanDoesNotOpenMicrophone(t *testing.T) {
	apps, err := NativeScan()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range apps {
		if app.ID == "" || app.PID <= 0 || app.Name == "" {
			t.Fatalf("invalid native application: %+v", app)
		}
	}
}

func TestClockChangeCannotCreateNegativeInterval(t *testing.T) {
	now := time.Now()
	apps := []Application{{ID: "meet", PID: 20}}
	m, err := Open(t.TempDir(), func() ([]Application, error) { return apps, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(now)
	m.Poll(now.Add(time.Second))
	m.Poll(now.Add(-time.Hour))
	apps = nil
	m.Poll(now.Add(-time.Hour + time.Second))
	usage := m.Snapshot().Items[0]
	if usage.EndedAt == nil || timeBefore(*usage.EndedAt, usage.StartedAt) || *usage.EndedAt != now.Add(time.Second).UTC().Format(time.RFC3339Nano) {
		t.Fatalf("clock change produced an invalid interval: %+v", usage)
	}
}

func TestSnapshotsRemainIndependentDuringLifecycleChanges(t *testing.T) {
	m, err := Open(t.TempDir(), func() ([]Application, error) { return []Application{{ID: "meet", PID: 20}}, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 50 {
			for _, item := range m.Snapshot().Items {
				if item.EndedAt != nil {
					*item.EndedAt = "modified copy"
				}
			}
		}
	})
	for i := range 10 {
		now := time.Now().Add(time.Duration(i) * time.Second)
		m.Resume()
		m.Poll(now)
		if err := m.Suspend(now); err != nil {
			t.Fatal(err)
		}
	}
	workers.Wait()
	for _, item := range m.Snapshot().Items {
		if item.EndedAt != nil && *item.EndedAt == "modified copy" {
			t.Fatal("snapshot mutation changed stored history")
		}
	}
}
