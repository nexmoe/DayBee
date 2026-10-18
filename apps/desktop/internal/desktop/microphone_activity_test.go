package desktop

import (
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/micactivity"
)

func TestMicrophoneSnapshotReusesHistoryAndRefreshesTransitions(t *testing.T) {
	apps := []micactivity.Application{{ID: "meeting", Name: "Meeting", PID: 42}}
	monitor, err := micactivity.Open(t.TempDir(), func() ([]micactivity.Application, error) { return apps, nil }, 99)
	if err != nil {
		t.Fatal(err)
	}
	s := &DesktopService{microphone: monitor}
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	monitor.Poll(now)
	first := s.refreshMicrophoneSnapshot()
	if len(first.Items) != 1 || first.Items[0].EndedAt != nil {
		t.Fatal("an observed app should appear active immediately after refresh")
	}
	for i := 0; i < 100; i++ {
		next := s.microphoneSnapshot()
		if &next.Items[0] != &first.Items[0] {
			t.Fatal("meter updates should reuse the saved microphone history")
		}
	}
	apps = nil
	monitor.Poll(now.Add(time.Second))
	next := s.refreshMicrophoneSnapshot()
	if next.Items[0].EndedAt == nil {
		t.Fatal("ending microphone use must update the cached view")
	}
	if first.Items[0].EndedAt != nil {
		t.Fatal("refresh must not mutate a snapshot already sent to a window")
	}
	apps = []micactivity.Application{{ID: "other", Name: "Other", PID: 43}}
	monitor.Poll(now.Add(2 * time.Second))
	if len(s.refreshMicrophoneSnapshot().Items) != 2 {
		t.Fatal("new app sessions must remain visible with the cache enabled")
	}
}
