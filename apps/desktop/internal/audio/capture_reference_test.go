package audio

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type referenceHandle struct {
	starts  int
	stops   int
	uninits int
	start   error
	fail    func(error)
}

func (h *referenceHandle) Start() error { h.starts++; return h.start }
func (h *referenceHandle) Stop() error  { h.stops++; return nil }
func (h *referenceHandle) Uninit()      { h.uninits++ }

func openReference(system *referenceHandle) func(func([]float32), func(error)) (captureDevice, error) {
	return func(_ func([]float32), onError func(error)) (captureDevice, error) {
		system.fail = onError
		return system, nil
	}
}

func TestSystemReferenceNotifiesOnceAndStopReleases(t *testing.T) {
	boom := errors.New("system reference unavailable")
	for _, runtime := range []bool{false, true} {
		name := "start"
		if runtime {
			name = "runtime"
		}
		t.Run(name, func(t *testing.T) {
			system := &referenceHandle{}
			if !runtime {
				system.start = boom
			}
			var notes []error
			mic := &referenceHandle{}
			capture := &NativeCapture{device: mic, expectedStop: &atomic.Bool{}}
			notify, err := capture.attachSystem(CaptureOptions{
				SystemReference:        true,
				OnReferenceUnavailable: func(err error) { notes = append(notes, err) },
			}, &atomic.Bool{}, capture.expectedStop, func(Packet) {}, nil, openReference(system))
			if err != nil || len(notes) != 0 || system.stops != 0 || system.uninits != 0 || mic.stops != 0 {
				t.Fatalf("attach stopped or notified early: err=%v notes=%d stops=%d", err, len(notes), system.stops)
			}
			if runtime {
				if notify != nil {
					t.Fatal("a running reference returned a start notice")
				}
				system.fail(boom)
				system.fail(boom)
			} else {
				if notify == nil {
					t.Fatal("reference start failure was returned as a capture error")
				}
				notify()
				notify()
			}
			if len(notes) != 1 || notes[0] != boom || system.stops != 0 || mic.stops != 0 {
				t.Fatalf("notes=%d stops=%d", len(notes), system.stops)
			}
			if err := capture.Stop(); err != nil {
				t.Fatal(err)
			}
			if err := capture.Stop(); err != nil {
				t.Fatal(err)
			}
			system.fail(boom)
			if len(notes) != 1 || system.stops != 1 || system.uninits != 1 || mic.stops != 1 || mic.uninits != 1 {
				t.Fatalf("Stop released system=%d/%d microphone=%d/%d notes=%d", system.stops, system.uninits, mic.stops, mic.uninits, len(notes))
			}
		})
	}
}

func TestReferenceStartNotifyRunsAfterUnlock(t *testing.T) {
	boom := errors.New("system reference unavailable")
	system := &referenceHandle{start: boom}
	mic := &referenceHandle{}
	capture := &NativeCapture{device: mic, expectedStop: &atomic.Bool{}}
	var notes int
	done := make(chan struct{})
	go func() {
		defer close(done)
		capture.mu.Lock()
		notify, err := capture.attachSystem(CaptureOptions{
			SystemReference: true,
			OnReferenceUnavailable: func(error) {
				notes++
				_ = capture.Stop()
			},
		}, &atomic.Bool{}, capture.expectedStop, func(Packet) {}, nil, openReference(system))
		if err != nil || notify == nil || notes != 0 {
			capture.mu.Unlock()
			return
		}
		capture.mu.Unlock()
		notify()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("start notice called Stop while the capture lock was held")
	}
	if notes != 1 || system.stops != 1 || system.uninits != 1 || mic.stops != 1 || mic.uninits != 1 {
		t.Fatalf("callback Stop released system=%d/%d microphone=%d/%d notes=%d", system.stops, system.uninits, mic.stops, mic.uninits, notes)
	}
	if err := capture.Stop(); err != nil || system.stops != 1 || mic.stops != 1 {
		t.Fatalf("repeated Stop changed system=%d microphone=%d err=%v", system.stops, mic.stops, err)
	}
}

func TestSystemOpenFailureDoesNotPanic(t *testing.T) {
	boom := errors.New("system capture failed to open")
	opens := []struct {
		name string
		open func(func([]float32), func(error)) (captureDevice, error)
		want error
	}{
		{"nil-error", func(func([]float32), func(error)) (captureDevice, error) { return nil, boom }, boom},
		{"nil-nil", func(func([]float32), func(error)) (captureDevice, error) { return nil, nil }, errSystemCapture},
	}
	for _, archived := range []bool{false, true} {
		for _, open := range opens {
			name := "reference/" + open.name
			if archived {
				name = "archive/" + open.name
			}
			t.Run(name, func(t *testing.T) {
				var notes []error
				mic := &referenceHandle{}
				capture := &NativeCapture{device: mic, expectedStop: &atomic.Bool{}}
				notify, err := capture.attachSystem(CaptureOptions{
					SystemAudio:            archived,
					SystemReference:        true,
					OnReferenceUnavailable: func(err error) { notes = append(notes, err) },
				}, &atomic.Bool{}, capture.expectedStop, func(Packet) {}, func(error) { notes = append(notes, errors.New("archive callback")) }, open.open)
				if len(notes) != 0 || mic.stops != 0 || capture.system != nil {
					t.Fatalf("open failure notified or retained a device: notes=%d", len(notes))
				}
				if archived {
					if notify != nil || !errors.Is(err, open.want) {
						t.Fatalf("archived open err=%v notify=%v", err, notify != nil)
					}
				} else if err != nil || notify == nil {
					t.Fatalf("reference open err=%v notify=%v", err, notify != nil)
				} else {
					notify()
					if len(notes) != 1 || !errors.Is(notes[0], open.want) {
						t.Fatalf("notes=%v", notes)
					}
				}
				if err := capture.Stop(); err != nil {
					t.Fatal(err)
				}
				if err := capture.Stop(); err != nil || mic.stops != 1 || mic.uninits != 1 {
					t.Fatalf("repeated Stop microphone=%d/%d err=%v", mic.stops, mic.uninits, err)
				}
			})
		}
	}
}

func TestSystemAudioFailureStaysStrict(t *testing.T) {
	boom := errors.New("system recording unavailable")
	system := &referenceHandle{start: boom}
	var notes int
	mic := &referenceHandle{}
	capture := &NativeCapture{device: mic, expectedStop: &atomic.Bool{}}
	notify, err := capture.attachSystem(CaptureOptions{
		SystemAudio:            true,
		SystemReference:        true,
		OnReferenceUnavailable: func(error) { notes++ },
	}, &atomic.Bool{}, capture.expectedStop, func(Packet) {}, func(error) { notes += 10 }, openReference(system))
	if notify != nil || !errors.Is(err, boom) || notes != 0 || system.stops != 0 || mic.stops != 0 {
		t.Fatalf("archived system audio was degraded: err=%v notes=%d stops=%d", err, notes, system.stops)
	}
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := capture.Stop(); err != nil || system.stops != 1 || system.uninits != 1 || mic.stops != 1 || mic.uninits != 1 {
		t.Fatalf("strict failure left system=%d/%d microphone=%d/%d", system.stops, system.uninits, mic.stops, mic.uninits)
	}
}

func TestSystemCaptureStaysOffWithoutRequest(t *testing.T) {
	capture := &NativeCapture{}
	called := false
	notify, err := capture.attachSystem(CaptureOptions{OnReferenceUnavailable: func(error) {
		t.Fatal("idle capture reported a reference failure")
	}}, &atomic.Bool{}, &atomic.Bool{}, func(Packet) {}, nil, func(func([]float32), func(error)) (captureDevice, error) {
		called = true
		return &referenceHandle{}, nil
	})
	if err != nil || notify != nil || called || capture.system != nil {
		t.Fatal("system capture started without SystemAudio or SystemReference")
	}
}
