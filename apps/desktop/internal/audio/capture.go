package audio

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

var ErrDisconnected = errors.New("Microphone disconnected.")
var ErrInterrupted = errors.New("Audio capture interrupted.")

type CaptureOptions struct {
	DeviceID         string
	EchoCancellation bool
	// SystemAudio must archive system playback. Its start and runtime failures abort capture.
	SystemAudio bool
	// SystemReference is a degradable, non-archived reference. A start failure is reported
	// after Start releases its lock. A runtime failure is reported from the system callback,
	// which does not hold that lock. The notice runs at most once. Stop or Close releases the handle.
	SystemReference        bool
	OnReferenceUnavailable func(error)
}

// errSystemCapture is the reference/archive failure used when open returns no device and no error.
var errSystemCapture = errors.New("system capture is unavailable")

type captureDevice interface {
	Start() error
	Stop() error
	Uninit()
}

type Packet struct {
	Source      protocol.AudioSource
	Samples     []float32
	DeviceID    string
	DeviceLabel string
	// CapturedAt is the arrival time stamped when the packet is queued.
	// Zero means the caller did not stamp it. This is not a hardware timestamp.
	CapturedAt time.Time
	// CaptureEpoch is the reference generation that queued the packet.
	// Zero means unstamped; the recorder accepts it as the current generation.
	CaptureEpoch uint64
}

type Capture interface {
	Devices() ([]protocol.DeviceInfo, error)
	Start(CaptureOptions, func(Packet), func(error)) error
	Stop() error
	Probe(context.Context, string) (float64, error)
	Close() error
}

// SwitchPolicy retains the browser controller's thresholds and hysteresis.
type SwitchPolicy struct {
	candidate  string
	wins       int
	cursor     int
	lastSwitch time.Time
}

// One returns the next candidate and remembers the rest for later polls.
func (p *SwitchPolicy) One(candidates []protocol.DeviceInfo) []protocol.DeviceInfo {
	if len(candidates) == 0 {
		return nil
	}
	device := candidates[p.cursor%len(candidates)]
	p.cursor++
	return []protocol.DeviceInfo{device}
}

func Candidates(devices []protocol.DeviceInfo, current, excluded string) []protocol.DeviceInfo {
	result := []protocol.DeviceInfo{}
	keywords := strings.Split(strings.ToLower(excluded), ",")
	for _, device := range devices {
		if device.ID == current {
			continue
		}
		ignore := false
		for _, keyword := range keywords {
			if keyword = strings.TrimSpace(keyword); keyword != "" && strings.Contains(strings.ToLower(device.Label), keyword) {
				ignore = true
			}
		}
		if !ignore {
			result = append(result, device)
		}
		if len(result) == 2 {
			break
		}
	}
	return result
}

func (p *SwitchPolicy) Consider(now time.Time, candidate string, best, current float64, confirmations int) bool {
	if candidate == "" || best < .006 || (current > 0 && best/current < 1.8) {
		p.candidate, p.wins = "", 0
		return false
	}
	if p.candidate != candidate {
		p.candidate, p.wins = candidate, 0
	}
	p.wins++
	if p.wins < max(1, confirmations) || now.Sub(p.lastSwitch) < 8*time.Second {
		return false
	}
	p.lastSwitch, p.candidate, p.wins = now, "", 0
	return true
}
