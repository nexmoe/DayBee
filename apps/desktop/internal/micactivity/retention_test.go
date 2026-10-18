package micactivity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestUnidentifiedProcessesAreNotArchived(t *testing.T) {
	directory := t.TempDir()
	m, err := Open(directory, func() ([]Application, error) {
		return []Application{{ID: "pid:14559", Name: "pid:14559", PID: 14559}, {ID: "zoom", Name: "Zoom", PID: 20}}, nil
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(time.Now())
	items := m.Snapshot().Items
	if len(items) != 1 || items[0].AppID != "zoom" {
		t.Fatalf("unidentified process was retained: %+v", items)
	}
}

func TestLoadingRemovesLegacyPIDEntriesFromDisk(t *testing.T) {
	directory := t.TempDir()
	at := "2026-10-09T10:00:00Z"
	write := func(day, appID string) {
		t.Helper()
		if err := config.WriteJSON(filepath.Join(directory, day+".json"), []entry{{MicrophoneUsage: protocol.MicrophoneUsage{
			ID: day, AppID: appID, AppName: appID, StartedAt: at, EndedAt: &at}}}); err != nil {
			t.Fatal(err)
		}
	}
	write("20261008", "pid:14559")
	write("20261009", "zoom")
	m, err := Open(directory, func() ([]Application, error) { return nil, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	if items := m.Snapshot().Items; len(items) != 1 || items[0].AppID != "zoom" {
		t.Fatalf("legacy process was loaded: %+v", items)
	}
	if _, err := os.Stat(filepath.Join(directory, "20261008.json")); !os.IsNotExist(err) {
		t.Fatalf("empty PID archive remains on disk: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "20261009.json"))
	if err != nil || !json.Valid(data) {
		t.Fatalf("identified history was damaged: %s %v", data, err)
	}
}

func TestRetentionPreservesRecordingContextAndActiveIndexes(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)
	apps := []Application{{ID: "unused", PID: 20}, {ID: "meeting", PID: 30}, {ID: "silence", PID: 40}, {ID: "system-only", PID: 50}, {ID: "active", PID: 60}}
	m, err := Open(directory, func() ([]Application, error) { return apps, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(old)
	apps = apps[4:]
	m.Poll(old.Add(time.Minute))
	apps = append(apps, Application{ID: "recent", PID: 70})
	m.Poll(now.Add(-time.Hour))
	// Only meeting overlaps saved microphone audio; system-only overlaps playback.
	rangeAt := func(start, end time.Time) []protocol.RecordingTimeRange {
		return []protocol.RecordingTimeRange{{StartedAt: start.Format(time.RFC3339Nano), EndedAt: end.Format(time.RFC3339Nano)}}
	}
	history := []protocol.RecordingHistoryItem{{AudioSource: protocol.SourceMicrophone,
		CapturedRanges: rangeAt(old.Add(10*time.Second), old.Add(20*time.Second))}}
	// Separate the other sessions from the saved audio while retaining their old age.
	for i := range m.entries {
		if m.entries[i].AppID != "meeting" && m.entries[i].EndedAt != nil {
			ended := old.Add(5 * time.Second).Format(time.RFC3339Nano)
			m.entries[i].EndedAt = &ended
		}
	}
	history = append(history, protocol.RecordingHistoryItem{AudioSource: protocol.SourceSystem,
		CapturedRanges: rangeAt(old.Add(time.Second), old.Add(2*time.Second))})
	if err := m.ReconcileHistory(history, now); err != nil {
		t.Fatal(err)
	}
	assertIDs := func(want map[string]bool) {
		t.Helper()
		items := m.Snapshot().Items
		if len(items) != len(want) {
			t.Fatalf("unexpected retained activity: %+v", items)
		}
		for _, item := range items {
			if !want[item.AppID] {
				t.Fatalf("retained unrelated activity: %+v", item)
			}
		}
	}
	assertIDs(map[string]bool{"meeting": true, "active": true, "recent": true})
	m.Poll(now) // Compaction must rebuild active session indexes.
	assertIDs(map[string]bool{"meeting": true, "active": true, "recent": true})
	if err := m.Close(now); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, func() ([]Application, error) { return nil, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Output directories may be switched or temporarily unavailable. A saved
	// association must survive restart and missing history in later snapshots.
	if err := reopened.ReconcileHistory(nil, now.Add(32*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items := reopened.Snapshot().Items
	if len(items) != 2 || items[0].AppID != "meeting" || items[1].AppID != "active" {
		t.Fatalf("recording context was lost after restart: %+v", items)
	}
}

func TestRecordingAssociationRespectsSilenceBoundariesAndAudioSource(t *testing.T) {
	at := func(seconds int) string {
		return time.Date(2026, 10, 9, 10, 0, seconds, 0, time.UTC).Format(time.RFC3339Nano)
	}
	ranges := []protocol.RecordingTimeRange{{StartedAt: at(10), EndedAt: at(15)}, {StartedAt: at(30), EndedAt: at(35)}}
	tests := []struct {
		name    string
		item    protocol.RecordingHistoryItem
		start   int
		end     int
		active  bool
		matches bool
	}{
		{"captured audio", protocol.RecordingHistoryItem{CapturedRanges: ranges}, 11, 14, false, true},
		{"silence", protocol.RecordingHistoryItem{CapturedRanges: ranges}, 16, 29, false, false},
		{"stop boundary", protocol.RecordingHistoryItem{CapturedRanges: ranges}, 5, 10, false, false},
		{"both boundaries", protocol.RecordingHistoryItem{CapturedRanges: ranges}, 15, 30, false, false},
		{"system playback", protocol.RecordingHistoryItem{AudioSource: protocol.SourceSystem, CapturedRanges: ranges}, 11, 14, false, false},
		{"legacy start", protocol.RecordingHistoryItem{StartedAt: at(10)}, 9, 11, false, true},
		{"legacy stop boundary", protocol.RecordingHistoryItem{StartedAt: at(10)}, 5, 10, false, false},
		{"active start point", protocol.RecordingHistoryItem{StartedAt: at(10)}, 10, 10, true, true},
		{"transcript fallback", protocol.RecordingHistoryItem{TranscriptChunks: []protocol.TranscriptChunk{{StartedAt: at(10), EndedAt: at(15)}}}, 11, 14, false, true},
		{"invalid range", protocol.RecordingHistoryItem{CapturedRanges: []protocol.RecordingTimeRange{{StartedAt: at(15), EndedAt: at(10)}}}, 11, 14, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, _ := time.Parse(time.RFC3339Nano, at(test.start))
			end, _ := time.Parse(time.RFC3339Nano, at(test.end))
			if got := overlapsRecording(recordedIntervals([]protocol.RecordingHistoryItem{test.item}), start, end, test.active); got != test.matches {
				t.Fatalf("recording association = %v, want %v", got, test.matches)
			}
		})
	}
	local, err := recordingTime("2026-10-09T10:00:00")
	if err != nil || !local.Equal(time.Date(2026, 10, 9, 10, 0, 0, 0, time.Local)) {
		t.Fatalf("legacy local time changed: %v %v", local, err)
	}
}

func TestPIDMigrationDoesNotWriteWhenAnotherArchiveIsCorrupt(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "20261008.json")
	at := "2026-10-08T10:00:00Z"
	if err := config.WriteJSON(path, []entry{{MicrophoneUsage: protocol.MicrophoneUsage{ID: "unknown", AppID: "pid:20", StartedAt: at, EndedAt: &at}}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := os.WriteFile(filepath.Join(directory, "20261009.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory, func() ([]Application, error) { return nil, nil }, 10); err == nil {
		t.Fatal("corrupt archive was ignored")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("migration changed archives despite a failed load")
	}
}

func TestRetentionRetriesFailedArchiveRemoval(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "activity")
	now := time.Now()
	apps := []Application{{ID: "unused", PID: 20}}
	m, err := Open(directory, func() ([]Application, error) { return apps, nil }, 10)
	if err != nil {
		t.Fatal(err)
	}
	m.Poll(now.Add(-32 * 24 * time.Hour))
	apps = nil
	m.Poll(now.Add(-31 * 24 * time.Hour))
	if err := os.Rename(directory, directory+"-offline"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ReconcileHistory(nil, now); err == nil || m.Snapshot().Error == nil {
		t.Fatal("failed archive removal was not reported")
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory+"-offline", directory); err != nil {
		t.Fatal(err)
	}
	if err := m.ReconcileHistory(nil, now); err != nil || m.Snapshot().Error != nil {
		t.Fatalf("archive removal did not retry: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(directory, "*.json"))
	if len(files) != 0 {
		t.Fatalf("expired archive remains on disk: %v", files)
	}
}
