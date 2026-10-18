package audio

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreMedia -framework AVFAudio -framework AudioToolbox -framework CoreAudio -framework Foundation
#include <stdlib.h>
#include "capture_system_darwin.h"
#include "capture_system_tap_darwin.h"
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

type systemCapture struct {
	consume func([]float32)
	onError func(error)
	native  unsafe.Pointer
	cancel  chan struct{}
	done    chan struct{}
}

func newSystemCapture(consume func([]float32), onError func(error)) (captureDevice, error) {
	return &systemCapture{consume: consume, onError: onError}, nil
}

func (c *systemCapture) Start() error {
	var message [512]C.char
	c.native = C.daybeeSystemOpen(&message[0], C.size_t(len(message)))
	if c.native == nil {
		return fmt.Errorf("System audio could not start: %s", C.GoString(&message[0]))
	}
	c.cancel, c.done = make(chan struct{}), make(chan struct{})
	go c.poll()
	return nil
}

func (c *systemCapture) poll() {
	defer close(c.done)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var buffer [2048]C.float
	for {
		select {
		case <-c.cancel:
			return
		case <-ticker.C:
			for {
				var failure C.int
				count := int(C.daybeeSystemRead(c.native, &buffer[0], C.uint32_t(len(buffer)), &failure))
				if failure != 0 {
					var message [1024]C.char
					C.daybeeSystemFailureDetail(c.native, &message[0], C.size_t(len(message)))
					err := systemFailureError(int(failure), C.GoString(&message[0]))
					if c.onError != nil {
						c.onError(err)
					}
					return
				}
				if count == 0 {
					break
				}
				samples := make([]float32, count)
				copy(samples, unsafe.Slice((*float32)(unsafe.Pointer(&buffer[0])), count))
				c.consume(samples)
			}
		}
	}
}

func (c *systemCapture) Stop() error {
	if c.native == nil {
		return nil
	}
	C.daybeeSystemStop(c.native)
	close(c.cancel)
	<-c.done
	C.daybeeSystemDestroy(c.native)
	c.native = nil
	return nil
}

func (c *systemCapture) Uninit() {}

// macOS 14.2+ opens System Audio Recording settings. Older releases still use
// ScreenCaptureKit, so they keep the Screen & System Audio Recording pane.
func SystemAudioSettingsURL() string {
	if C.daybeeSystemTapAtLeast14_2() != 0 {
		return "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AudioCapture"
	}
	return "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
}
