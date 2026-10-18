package recorder

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type fakeCapture struct {
	mu         sync.Mutex
	callback   func(audio.Packet)
	fail       func(error)
	closed     bool
	failSystem bool
	failID     string
	options    []audio.CaptureOptions
}

func (c *fakeCapture) Devices() ([]protocol.DeviceInfo, error) {
	return []protocol.DeviceInfo{{ID: "default", IsDefault: true, Label: "Test Microphone"}}, nil
}
func (c *fakeCapture) Start(options audio.CaptureOptions, callback func(audio.Packet), fail func(error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.options = append(c.options, options)
	if c.failSystem && options.SystemAudio {
		return errors.New("system audio permission denied")
	}
	if c.failID != "" && options.DeviceID == c.failID {
		return errors.New("test microphone unavailable")
	}
	c.callback = callback
	c.fail = fail
	return nil
}
func (c *fakeCapture) Stop() error                                    { c.mu.Lock(); defer c.mu.Unlock(); c.callback = nil; return nil }
func (c *fakeCapture) Probe(context.Context, string) (float64, error) { return 0, nil }
func (c *fakeCapture) Close() error                                   { c.mu.Lock(); defer c.mu.Unlock(); c.closed = true; return nil }
func (c *fakeCapture) emit(samples []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.callback != nil {
		c.callback(audio.Packet{Samples: samples, DeviceID: "default", DeviceLabel: "Test Microphone"})
	}
}

type fakeAssets struct {
	mu           sync.Mutex
	asrRequested bool
	failFLAC     bool
	read         func(context.Context, string) ([]float32, int, error)
	selected     protocol.ASRModel
}

func (a *fakeAssets) Ensure(_ context.Context, _ bool, asr bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asrRequested = a.asrRequested || asr
	return nil
}
func (a *fakeAssets) VADPath() string { return "vad" }
func (a *fakeAssets) SelectASRModel(id protocol.ASRModel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.selected = id
}
func (a *fakeAssets) ASRPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ASRModelPath(a.selected)
}
func (a *fakeAssets) ASRModelPath(id protocol.ASRModel) string { return string(id) }
func (a *fakeAssets) EnsureASRModel(ctx context.Context, _ protocol.ASRModel) error {
	return a.Ensure(ctx, false, true)
}
func (a *fakeAssets) Ready() (bool, bool) { return true, true }
func (a *fakeAssets) Transcode(context.Context, string, string) error {
	return errors.New("conversion failed")
}
func (a *fakeAssets) ReadAudio(ctx context.Context, path string) ([]float32, int, error) {
	if a.read != nil {
		return a.read(ctx, path)
	}
	return []float32{.1}, 16000, nil
}

type fakeRuntime struct {
	hold        bool
	gate        chan struct{}
	entered     chan struct{}
	mu          sync.Mutex
	recognizers int
	releases    int
	vadsClosed  int
	modelText   bool
}

func (r *fakeRuntime) NewRecognizer(model string) (asr.Recognizer, error) {
	r.mu.Lock()
	r.recognizers++
	r.mu.Unlock()
	return &testRecognizer{runtime: r, model: model}, nil
}
func (r *fakeRuntime) NewVAD(string) (asr.VAD, error)          { return &testVAD{runtime: r}, nil }
func (r *fakeRuntime) ReadWave(string) ([]float32, int, error) { return []float32{.1}, 16000, nil }

type testRecognizer struct {
	runtime *fakeRuntime
	model   string
}

func (r *testRecognizer) Decode([]float32, int) (asr.Result, error) {
	if r.runtime.entered != nil {
		select {
		case r.runtime.entered <- struct{}{}:
		default:
		}
	}
	if r.runtime.gate != nil {
		<-r.runtime.gate
	}
	if r.runtime.modelText {
		return asr.Result{Text: r.model, Language: "zh"}, nil
	}
	return asr.Result{Text: "固定测试文本", Language: "zh"}, nil
}
func (r *testRecognizer) Close() { r.runtime.mu.Lock(); r.runtime.releases++; r.runtime.mu.Unlock() }

type testVAD struct {
	runtime *fakeRuntime
	queued  []asr.Speech
	offset  int
	flushed bool
}

func (v *testVAD) Accept(s []float32) {
	v.queued = append(v.queued, asr.Speech{Start: v.offset, Samples: append([]float32{}, s...)})
	v.offset += len(s)
}
func (v *testVAD) Detected() bool { return len(v.queued) > 0 }
func (v *testVAD) Pop() asr.Speech {
	if len(v.queued) == 0 || (v.runtime.hold && !v.flushed) {
		return asr.Speech{}
	}
	s := v.queued[0]
	v.queued = v.queued[1:]
	return s
}
func (v *testVAD) Flush() { v.flushed = true }
func (v *testVAD) Reset() { v.queued = nil; v.flushed = false; v.offset = 0 }
func (v *testVAD) Close() { v.runtime.mu.Lock(); v.runtime.vadsClosed++; v.runtime.mu.Unlock() }

func testEngine(t *testing.T, disabled bool, runtime *fakeRuntime) (*Engine, *fakeCapture, *fakeAssets, string) {
	t.Helper()
	directory := t.TempDir()
	settings := protocol.DefaultSettings(directory)
	settings.Recording.DisableASR = disabled
	settings.Recording.AudioFormat = protocol.WAV
	settings.Recording.AutoSwitchDevice = false
	capture, assets := &fakeCapture{}, &fakeAssets{}
	e := New(settings, capture, assets, runtime, nil, nil)
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e, capture, assets, directory
}
