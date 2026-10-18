package audio

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestRawProbesKeepVoiceCaptureRunning(t *testing.T) {
	if os.Getenv("DAYBEE_TEST_MICROPHONE") != "1" {
		t.Skip("Set DAYBEE_TEST_MICROPHONE=1 to verify concurrent raw probes and live AEC")
	}
	capture, err := NewNativeCapture()
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	devices, err := capture.Devices()
	if err != nil {
		t.Fatal(err)
	}
	var samples atomic.Int64
	failures := make(chan error, 4)
	if err := capture.Start(CaptureOptions{DeviceID: "default", EchoCancellation: true}, func(packet Packet) {
		samples.Add(int64(len(packet.Samples)))
	}, func(err error) {
		select {
		case failures <- err:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	probes := 0
	for _, device := range devices {
		if !device.IsDefault {
			continue
		}
		for range 2 {
			before := samples.Load()
			if _, err := capture.Probe(context.Background(), device.ID); err != nil {
				t.Fatal(err)
			}
			probes++
			if samples.Load() <= before {
				t.Fatal("raw probe interrupted live voice capture")
			}
		}
	}
	if probes == 0 {
		t.Fatal("no default microphone was probed")
	}
	before := samples.Load()
	time.Sleep(300 * time.Millisecond)
	if samples.Load() <= before {
		t.Fatal("AEC capture did not continue after probes")
	}
	if err := capture.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	t.Logf("Live AEC delivered %d samples through %d concurrent raw probes", samples.Load(), probes)
}
