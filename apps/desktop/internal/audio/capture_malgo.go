package audio

// #include <stdlib.h>
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type NativeCapture struct {
	mu           sync.Mutex
	context      *malgo.AllocatedContext
	device       captureDevice
	system       captureDevice
	expectedStop *atomic.Bool
}

func NewNativeCapture() (*NativeCapture, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		slog.Debug("miniaudio", "message", message)
	})
	if err != nil {
		return nil, fmt.Errorf("initialize audio devices: %w", err)
	}
	return &NativeCapture{context: ctx}, nil
}

func nativeID(info malgo.DeviceInfo) string { return "native:" + info.ID.String() }

func (c *NativeCapture) Devices() ([]protocol.DeviceInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.context == nil {
		return nil, errors.New("audio devices are closed")
	}
	infos, err := c.context.Devices(malgo.Capture)
	if err != nil {
		return nil, err
	}
	result := make([]protocol.DeviceInfo, 0, len(infos))
	for i, info := range infos {
		result = append(result, protocol.DeviceInfo{ID: nativeID(info), Index: i, IsDefault: info.IsDefault != 0, Label: info.Name()})
	}
	return result, nil
}

func (c *NativeCapture) initialize(id string, echoCancellation bool, consume func([]float32), onError func(error)) (captureDevice, protocol.DeviceInfo, error) {
	if c.context == nil {
		return nil, protocol.DeviceInfo{}, errors.New("audio devices are closed")
	}
	infos, err := c.context.Devices(malgo.Capture)
	if err != nil {
		return nil, protocol.DeviceInfo{}, err
	}
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format, config.Capture.Channels = malgo.FormatF32, 1
	config.SampleRate, config.PeriodSizeInFrames = SampleRate, WindowSize
	selected := protocol.DeviceInfo{ID: "default", Label: "default", IsDefault: true}
	var uid malgo.DeviceID
	found := id == "default"
	for _, info := range infos {
		if (id == "default" && info.IsDefault != 0) || id == nativeID(info) {
			selected = protocol.DeviceInfo{ID: nativeID(info), Label: info.Name(), IsDefault: info.IsDefault != 0}
			if id != "default" {
				uid = info.ID
				config.Capture.DeviceID = info.ID.Pointer()
				defer C.free(config.Capture.DeviceID)
			}
			found = true
			break
		}
	}
	if !found {
		return nil, selected, fmt.Errorf("Microphone is unavailable: %s", id)
	}
	if echoCancellation {
		device, err := newVoiceCapture(uid, consume, onError)
		return device, selected, err
	}
	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			if len(input) > 0 {
				consume(decodeFloat32(input))
			}
		},
		Stop: func() {
			if onError != nil {
				onError(ErrDisconnected)
			}
		},
	}
	device, err := malgo.InitDevice(c.context.Context, config, callbacks)
	if err != nil {
		return nil, selected, fmt.Errorf("open microphone: %w", err)
	}
	if device.SampleRate() != SampleRate || device.CaptureChannels() != 1 || device.CaptureFormat() != malgo.FormatF32 {
		device.Uninit()
		return nil, selected, errors.New("microphone could not provide 16 kHz mono Float32 audio")
	}
	return device, selected, nil
}

func decodeFloat32(input []byte) []float32 {
	samples := make([]float32, len(input)/4)
	for i := range samples {
		value := math.Float32frombits(binary.LittleEndian.Uint32(input[i*4:]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			value = 0
		}
		samples[i] = value
	}
	return samples
}

func (c *NativeCapture) Start(options CaptureOptions, consume func(Packet), onError func(error)) error {
	notify, err := c.start(options, consume, onError)
	if notify != nil {
		notify()
	}
	return err
}

func (c *NativeCapture) start(options CaptureOptions, consume func(Packet), onError func(error)) (func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stopLocked(); err != nil {
		return nil, err
	}
	intentional := &atomic.Bool{}
	ready := &atomic.Bool{}
	var selected protocol.DeviceInfo
	device, info, err := c.initialize(options.DeviceID, options.EchoCancellation, func(samples []float32) {
		if ready.Load() && !intentional.Load() {
			consume(Packet{Samples: samples, DeviceID: selected.ID, DeviceLabel: selected.Label, Source: protocol.SourceMicrophone})
		}
	}, func(err error) {
		if !intentional.Load() && onError != nil {
			onError(err)
		}
	})
	if err != nil {
		return nil, err
	}
	selected = info
	c.device, c.expectedStop = device, intentional
	notify, err := c.attachSystem(options, ready, intentional, consume, onError, newSystemCapture)
	if err != nil {
		_ = c.stopLocked()
		return nil, err
	}
	if err := device.Start(); err != nil {
		_ = c.stopLocked()
		return notify, fmt.Errorf("start microphone: %w", err)
	}
	ready.Store(true)
	return notify, nil
}

// attachSystem starts loopback only when SystemAudio or SystemReference is set.
// Archived capture returns its error. A reference start failure returns a notify
// func and leaves any system handle installed until Stop or Close.
func (c *NativeCapture) attachSystem(options CaptureOptions, ready *atomic.Bool, intentional *atomic.Bool, consume func(Packet), onError func(error), open func(func([]float32), func(error)) (captureDevice, error)) (func(), error) {
	if !options.SystemAudio && !options.SystemReference {
		return nil, nil
	}
	var once sync.Once
	notify := func(err error) {
		if err == nil || options.SystemAudio || options.OnReferenceUnavailable == nil {
			return
		}
		once.Do(func() { options.OnReferenceUnavailable(err) })
	}
	system, err := open(func(samples []float32) {
		if ready.Load() && !intentional.Load() {
			consume(Packet{Samples: samples, DeviceID: "system", DeviceLabel: "System audio", Source: protocol.SourceSystem})
		}
	}, func(err error) {
		if intentional.Load() {
			return
		}
		if options.SystemAudio {
			if onError != nil {
				onError(err)
			}
			return
		}
		notify(err)
	})
	if err == nil && system == nil {
		err = errSystemCapture
	}
	if err == nil {
		err = system.Start()
	}
	if system != nil {
		c.system = system
	}
	if err == nil {
		return nil, nil
	}
	if options.SystemAudio {
		return nil, err
	}
	failure := err
	return func() { notify(failure) }, nil
}

func (c *NativeCapture) stopLocked() error {
	if c.device == nil {
		return nil
	}
	c.expectedStop.Store(true)
	err := c.device.Stop()
	if c.system != nil {
		err = errors.Join(err, c.system.Stop())
		c.system.Uninit()
		c.system = nil
	}
	c.device.Uninit()
	c.device, c.expectedStop = nil, nil
	return err
}

func (c *NativeCapture) Stop() error { c.mu.Lock(); defer c.mu.Unlock(); return c.stopLocked() }

func (c *NativeCapture) Probe(ctx context.Context, id string) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sum float64
	var count int
	packets := make(chan []float32, 8)
	device, _, err := c.initialize(id, false, func(samples []float32) {
		// Device callbacks only copy into a bounded queue, including probes.
		select {
		case packets <- samples:
		default:
		}
	}, nil)
	if err != nil {
		return 0, err
	}
	defer device.Uninit()
	if err := device.Start(); err != nil {
		return 0, err
	}
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	accumulate := func(samples []float32) {
		for _, sample := range samples {
			sum += float64(sample) * float64(sample)
			count++
		}
	}
	for {
		select {
		case <-ctx.Done():
			_ = device.Stop()
			return 0, ctx.Err()
		case samples := <-packets:
			accumulate(samples)
		case <-timer.C:
			if err := device.Stop(); err != nil {
				return 0, err
			}
			for {
				select {
				case samples := <-packets:
					accumulate(samples)
				default:
					if count == 0 {
						return 0, nil
					}
					return math.Sqrt(sum / float64(count)), nil
				}
			}
		}
	}
}

func (c *NativeCapture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.stopLocked()
	if c.context != nil {
		err = errors.Join(err, c.context.Uninit())
		c.context.Free()
		c.context = nil
	}
	return err
}
