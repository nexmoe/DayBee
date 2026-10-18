package audio

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework AVFAudio -framework AudioToolbox -framework CoreAudio -framework Foundation
#include <stdlib.h>
#include "capture_voice_darwin.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

type voiceCapture struct {
	uid     string
	consume func([]float32)
	onError func(error)
	// Experimental: native VAD alone still admits some speaker playback.
	// Keep it opt-in for acceptance tests until a playback reference is used.
	detectSpeech bool
	// Optional acceptance-test observer. Ordinary capture retains no ungated audio.
	inspect func([]float32, int)
	native  unsafe.Pointer
	cancel  chan struct{}
	done    chan struct{}
}

func newVoiceCapture(id malgo.DeviceID, consume func([]float32), onError func(error)) (captureDevice, error) {
	// miniaudio's Core Audio ID contains the UTF-8 device UID, zero-padded.
	uid := bytes.TrimRight(id[:], "\x00")
	return &voiceCapture{uid: string(uid), consume: consume, onError: onError}, nil
}

func (c *voiceCapture) Start() error {
	uid := C.CString(c.uid)
	defer C.free(unsafe.Pointer(uid))
	var message [512]C.char
	var detectSpeech C.int
	if c.detectSpeech {
		detectSpeech = 1
	}
	c.native = C.daybeeVoiceOpen(uid, detectSpeech, &message[0], C.size_t(len(message)))
	if c.native == nil {
		return fmt.Errorf("Echo cancellation could not start: %s. Check the microphone or turn off Echo cancellation in Input settings", C.GoString(&message[0]))
	}
	c.cancel, c.done = make(chan struct{}), make(chan struct{})
	go c.poll()
	return nil
}

func (c *voiceCapture) speechActivity() int {
	if !c.detectSpeech {
		return -1
	}
	return int(C.daybeeVoiceSpeechActivity(c.native))
}

func (c *voiceCapture) poll() {
	defer close(c.done)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var buffer [2048]C.float
	gate := voiceActivityGate{}
	lastActivity := -1
	consume := func(samples []float32) {
		if len(samples) > 0 {
			c.consume(samples)
		}
	}
	// Preserve already captured speech even when a disconnect ends polling.
	// This runs before done closes, so Stop still waits for the final packet.
	defer func() { consume(gate.Flush()) }()
	drain := func(activity int) bool {
		for {
			var failure C.int
			count := int(C.daybeeVoiceRead(c.native, &buffer[0], C.uint32_t(len(buffer)), &failure))
			if failure != 0 {
				err := ErrDisconnected
				switch failure {
				case 2:
					err = errors.New("Echo cancellation audio buffer overflow. Recording was stopped safely.")
				case 3:
					err = errors.New("Echo cancellation audio conversion failed. Recording was stopped safely.")
				}
				if c.onError != nil {
					c.onError(err)
				}
				return false
			}
			if count == 0 {
				return true
			}
			samples := unsafe.Slice((*float32)(unsafe.Pointer(&buffer[0])), count)
			if c.inspect != nil {
				c.inspect(samples, activity)
			}
			if c.detectSpeech {
				consume(gate.Push(samples, activity))
			} else {
				consume(append([]float32(nil), samples...))
			}
		}
	}
	for {
		select {
		case <-c.cancel:
			drain(lastActivity)
			return
		case <-ticker.C:
			lastActivity = c.speechActivity()
			if !drain(lastActivity) {
				return
			}
		}
	}
}

func (c *voiceCapture) Stop() error {
	if c.native == nil {
		return nil
	}
	C.daybeeVoiceStop(c.native)
	close(c.cancel)
	<-c.done
	C.daybeeVoiceDestroy(c.native)
	c.native = nil
	return nil
}

func (c *voiceCapture) Uninit() {}
