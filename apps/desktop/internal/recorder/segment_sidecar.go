package recorder

import (
	"errors"
	"sync"
)

// recordingSidecar writes the latest scene metadata off the audio goroutine.
// finish waits for that writer, then writes once more so stop cannot return
// before the last ranges are on disk.
type recordingSidecar struct {
	seg     *segment
	mu      sync.Mutex
	cond    *sync.Cond
	path    string
	dirty   bool
	running bool
	closed  bool
	err     error
}

func newRecordingSidecar(seg *segment) *recordingSidecar {
	s := &recordingSidecar{seg: seg}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *recordingSidecar) queue(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.path = path
	s.dirty = true
	if !s.running {
		s.running = true
		go s.loop()
	}
	s.cond.Signal()
}

func (s *recordingSidecar) loop() {
	for {
		s.mu.Lock()
		for !s.dirty && !s.closed {
			s.cond.Wait()
		}
		if !s.dirty || s.closed {
			s.running = false
			s.cond.Broadcast()
			s.mu.Unlock()
			return
		}
		path := s.path
		s.dirty = false
		s.mu.Unlock()
		if err := writeRecordingMetadata(s.seg, path); err != nil {
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
		}
	}
}

func (s *recordingSidecar) finish(path string) error {
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast()
	for s.running {
		s.cond.Wait()
	}
	err := s.err
	s.mu.Unlock()
	return errors.Join(err, writeRecordingMetadata(s.seg, path))
}
