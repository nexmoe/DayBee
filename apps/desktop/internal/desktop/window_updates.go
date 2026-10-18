package desktop

import (
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/recorder"
)

type windowUpdateSink struct {
	visible   bool
	publisher snapshotPublisher
	snapshot  func(protocol.DesktopSnapshot)
	status    func(protocol.DesktopStatusSnapshot)
	waveform  func(protocol.WaveformFrame)
}

// Native visibility events update this cache. The audio goroutine never
// queries AppKit/WebView or wakes a hidden page to ask whether it is visible.
type windowUpdates struct {
	mu     sync.Mutex
	sinks  map[string]*windowUpdateSink
	active atomic.Bool
}

func (u *windowUpdates) register(role string, sink *windowUpdateSink) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.sinks == nil {
		u.sinks = make(map[string]*windowUpdateSink)
	}
	u.sinks[role] = sink
}

func (u *windowUpdates) setVisible(role string, visible bool) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	sink := u.sinks[role]
	if sink == nil || sink.visible == visible {
		return false
	}
	sink.visible = visible
	// A reopened window needs all changes it missed, even if another window
	// has already received those changes. Bootstrap still handles page reloads.
	sink.publisher = snapshotPublisher{}
	active := false
	for _, sink := range u.sinks {
		active = active || sink.visible
	}
	u.active.Store(active)
	return true
}

func (u *windowUpdates) publish(snapshot protocol.DesktopSnapshot, historyVersion uint64) {
	var sends []func()
	u.mu.Lock()
	for _, sink := range u.sinks {
		if !sink.visible {
			continue
		}
		switch sink.publisher.next(snapshot, historyVersion) {
		case publishFullSnapshot:
			sends = append(sends, func() { sink.snapshot(snapshot) })
		case publishStatusSnapshot:
			status := protocol.DesktopStatusSnapshot{Status: snapshot.Status, Voiceprint: snapshot.Voiceprint}
			sends = append(sends, func() { sink.status(status) })
		}
	}
	u.mu.Unlock()
	// Sending can cross onto the main thread. Do not hold the visibility lock
	// there: a native hide/show callback may need it at the same time.
	for _, send := range sends {
		send()
	}
}

func (u *windowUpdates) waveform(frame protocol.WaveformFrame) {
	var sends []func(protocol.WaveformFrame)
	u.mu.Lock()
	for _, sink := range u.sinks {
		if sink.visible {
			sends = append(sends, sink.waveform)
		}
	}
	u.mu.Unlock()
	for _, send := range sends {
		send(frame)
	}
}

func (s *DesktopService) trackWindowUpdates(role string, window *mygo.Window) {
	s.windows.register(role, &windowUpdateSink{
		snapshot: func(snapshot protocol.DesktopSnapshot) { _ = SnapshotEvent.Emit(window, snapshot) },
		status:   func(status protocol.DesktopStatusSnapshot) { _ = StatusEvent.Emit(window, status) },
		waveform: func(frame protocol.WaveformFrame) { _ = WaveformEvent.Emit(window, frame) },
	})
	update := func(visible bool) {
		if s.windows.setVisible(role, visible) {
			s.markDirty()
		}
		if e, ok := s.engine.(*recorder.Engine); ok {
			e.SetWaveformEnabled(s.windows.active.Load())
		}
	}
	refresh := func() {
		// Background acceptance tests intentionally render hidden windows.
		update(s.backgroundTest() || (window.IsVisible() && !window.IsMinimized() && !mygo.App.IsHidden()))
	}
	window.OnShow(refresh)
	window.OnHide(refresh)
	window.OnMinimize(refresh)
	window.OnRestore(refresh)
	window.OnFocus(refresh)
	window.OnBlur(refresh)
	window.OnClosed(func() { update(false) })
	mygo.App.OnActivate(func(bool) { refresh() })
	refresh()
}
