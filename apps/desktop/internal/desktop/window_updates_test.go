package desktop

import (
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

type windowUpdateCounts struct {
	full, status, waveform int
	last                   protocol.DesktopSnapshot
}

func countWindowUpdates(u *windowUpdates, role string) *windowUpdateCounts {
	c := &windowUpdateCounts{}
	u.register(role, &windowUpdateSink{
		snapshot: func(snapshot protocol.DesktopSnapshot) { c.full++; c.last = snapshot },
		status:   func(protocol.DesktopStatusSnapshot) { c.status++ },
		waveform: func(protocol.WaveformFrame) { c.waveform++ },
	})
	return c
}

func TestHiddenWindowsSkipUpdatesAndReceiveFreshStateOnShow(t *testing.T) {
	var u windowUpdates
	compact, main := countWindowUpdates(&u, "compact"), countWindowUpdates(&u, "main")
	snapshot := publicationFixture()
	u.publish(snapshot, 1)
	u.waveform(protocol.WaveformFrame{})
	if u.active.Load() || compact.full != 0 || main.full != 0 || compact.waveform != 0 || main.waveform != 0 {
		t.Fatal("hidden windows received updates")
	}
	u.setVisible("compact", true)
	u.publish(snapshot, 1)
	snapshot.Status.Elapsed = "00:00:01"
	u.publish(snapshot, 1)
	u.waveform(protocol.WaveformFrame{})
	if !u.active.Load() || compact.full != 1 || compact.status != 1 || compact.waveform != 1 || main.full != 0 {
		t.Fatal("visible window did not receive full then lightweight updates")
	}
	u.setVisible("main", true)
	u.publish(snapshot, 1)
	if main.full != 1 || compact.full != 1 {
		t.Fatal("new window needs a full snapshot without refreshing the other one")
	}
	u.setVisible("compact", false)
	snapshot.Settings.Desktop.Language = protocol.LanguageChinese
	snapshot.History = append(snapshot.History, protocol.RecordingHistoryItem{ID: "speech-recorded-while-hidden"})
	u.publish(snapshot, 2)
	u.waveform(protocol.WaveformFrame{})
	if compact.full != 1 || compact.waveform != 1 || main.full != 2 || main.waveform != 1 {
		t.Fatal("updates were sent to a hidden window or withheld from a visible one")
	}
	u.setVisible("compact", true)
	u.publish(snapshot, 2)
	if compact.full != 2 || len(compact.last.History) != len(snapshot.History) || compact.last.Settings != snapshot.Settings {
		t.Fatal("reopened window missed settings or speech recorded while it was hidden")
	}
	u.setVisible("compact", false)
	u.setVisible("main", false)
	for range 100 {
		snapshot.Status.Elapsed = "00:10:00"
		u.publish(snapshot, 2)
		u.waveform(protocol.WaveformFrame{})
	}
	if u.active.Load() || compact.status != 1 || main.status != 0 || main.waveform != 1 {
		t.Fatal("background updates woke hidden windows")
	}
}

func TestWindowSendDoesNotHoldVisibilityLock(t *testing.T) {
	var u windowUpdates
	u.register("compact", &windowUpdateSink{snapshot: func(protocol.DesktopSnapshot) { u.setVisible("compact", false) }})
	u.setVisible("compact", true)
	u.publish(publicationFixture(), 1)
	if u.active.Load() {
		t.Fatal("visibility change during send was lost")
	}
}

func BenchmarkWindowPublicationHidden(b *testing.B) {
	var u windowUpdates
	countWindowUpdates(&u, "compact")
	countWindowUpdates(&u, "main")
	snapshot := publicationFixture()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		u.publish(snapshot, 1)
	}
}
