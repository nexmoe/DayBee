package desktop

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/micactivity"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func (s *DesktopService) initializeMicrophoneActivity(root string) {
	if s.test {
		return
	}
	monitor, err := micactivity.Open(filepath.Join(root, "microphone-activity"), micactivity.NativeScan, os.Getpid())
	if err != nil {
		message := err.Error()
		s.microphoneError = &message
		slog.Error("Microphone activity initialization", "error", err)
		return
	}
	s.microphone = monitor
	monitor.Poll(time.Now())
	s.refreshMicrophoneSnapshot()
}

func (s *DesktopService) microphoneActivityLoop() {
	if s.microphone == nil {
		return
	}
	// Polling also works on macOS versions where per-process property listeners
	// do not fire. Accuracy is bounded by this one-second observation interval.
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var lastError string
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-tick.C:
			before := s.microphoneSnapshot()
			s.microphone.Poll(now)
			snapshot := s.refreshMicrophoneSnapshot()
			if snapshot.Error != nil {
				if *snapshot.Error != lastError {
					slog.Error("Microphone activity", "error", *snapshot.Error)
				}
				lastError = *snapshot.Error
			} else {
				lastError = ""
			}
			if !reflect.DeepEqual(before, snapshot) {
				s.markDirty()
			}
		}
	}
}

// Monitoring and history reconciliation own refreshes. Audio meter updates
// reuse this immutable view instead of copying and sorting every saved session.
func (s *DesktopService) refreshMicrophoneSnapshot() protocol.MicrophoneActivitySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.microphoneActivity = s.microphone.Snapshot()
	return s.microphoneActivity
}

func (s *DesktopService) microphoneSnapshot() protocol.MicrophoneActivitySnapshot {
	if s.microphone != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.microphoneActivity
	}
	return protocol.MicrophoneActivitySnapshot{Supported: s.test || s.microphoneError != nil,
		Error: s.microphoneError, Items: []protocol.MicrophoneUsage{}}
}
