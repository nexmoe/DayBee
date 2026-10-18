package recorder

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

type autoSwitchCapture struct {
	*fakeCapture
	levels      map[string]float64
	probeErrors map[string]error
	probes      []string
}

func (c *autoSwitchCapture) Probe(_ context.Context, id string) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probes = append(c.probes, id)
	return c.levels[id], c.probeErrors[id]
}

func autoSwitchEngine(t *testing.T, levels map[string]float64) (*Engine, *autoSwitchCapture) {
	t.Helper()
	s := protocol.DefaultSettings(t.TempDir())
	s.Recording.AutoSwitchDevice, s.Recording.EchoCancellation, s.Recording.RecordSystemAudio = true, true, true
	s.Recording.DisableASR = true
	s.Recording.AudioFormat = protocol.WAV
	c := &autoSwitchCapture{fakeCapture: &fakeCapture{}, levels: levels, probeErrors: map[string]error{}}
	e := New(s, c, &fakeAssets{}, &fakeRuntime{}, nil, nil)
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	return e, c
}

var switchDevices = []protocol.DeviceInfo{
	{ID: "default", Label: "Current microphone", IsDefault: true},
	{ID: "usb", Label: "USB microphone"},
	{ID: "iphone", Label: "iPhone microphone"},
}

func TestAutoSwitchWithEchoCancellationKeepsProcessingAndSystemAudio(t *testing.T) {
	e, capture := autoSwitchEngine(t, map[string]float64{"default": .01, "usb": .04, "iphone": .9})
	policy, now := &audio.SwitchPolicy{}, time.Now()
	s := e.Settings().Recording
	// The processed live level must not be compared with a raw candidate.
	e.considerAutoSwitch(policy, s, "default", switchDevices, .9, now)
	capture.mu.Lock()
	starts := len(capture.options)
	capture.mu.Unlock()
	if starts != 1 {
		t.Fatal("switched before the configured two confirmations")
	}
	e.considerAutoSwitch(policy, s, "default", switchDevices, .9, now.Add(3*time.Second))
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 2 {
		t.Fatalf("AEC prevented automatic switching: %+v", capture.options)
	}
	last := capture.options[1]
	if last.DeviceID != "usb" || !last.EchoCancellation || !last.SystemAudio || !e.Status().Recording {
		t.Fatalf("automatic switching lost processing or sources: %+v", last)
	}
	if !reflect.DeepEqual(capture.probes, []string{"default", "usb", "default", "usb"}) {
		t.Fatalf("inconsistent raw comparison or ignored keyword: %+v", capture.probes)
	}
}

func TestAutoSwitchDoesNotMistakeAECAttenuationForStrongerInput(t *testing.T) {
	e, capture := autoSwitchEngine(t, map[string]float64{"default": .04, "usb": .03})
	policy, now := &audio.SwitchPolicy{}, time.Now()
	for i := range 3 {
		e.considerAutoSwitch(policy, e.Settings().Recording, "default", switchDevices, .001, now.Add(time.Duration(i)*3*time.Second))
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 1 {
		t.Fatal("lower processed RMS caused a false switch")
	}
}

func TestAutoSwitchProbesOneCandidatePerPass(t *testing.T) {
	e, capture := autoSwitchEngine(t, map[string]float64{"default": .01, "usb": .9, "iphone": .8})
	s := e.Settings()
	s.Recording.ExcludeDeviceKeywords = ""
	s.Recording.EchoCancellation = false
	if err := e.Apply(s); err != nil {
		t.Fatal(err)
	}
	policy, now := &audio.SwitchPolicy{}, time.Now()
	recording := e.Settings().Recording
	e.considerAutoSwitch(policy, recording, "default", switchDevices, .001, now)
	e.considerAutoSwitch(policy, recording, "default", switchDevices, .001, now.Add(10*time.Second))
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !reflect.DeepEqual(capture.probes, []string{"usb", "iphone"}) {
		t.Fatalf("probed more than one candidate per pass: %+v", capture.probes)
	}
}

func TestAutoSwitchSkipsUnreliableCurrentProbeAndRespectsDisabledPreference(t *testing.T) {
	e, capture := autoSwitchEngine(t, map[string]float64{"default": .01, "usb": .9})
	capture.mu.Lock()
	capture.probeErrors["default"] = errors.New("unavailable")
	capture.mu.Unlock()
	policy, now := &audio.SwitchPolicy{}, time.Now()
	for i := range 3 {
		e.considerAutoSwitch(policy, e.Settings().Recording, "default", switchDevices, .001, now.Add(time.Duration(i)*3*time.Second))
	}
	s := e.Settings()
	s.Recording.AutoSwitchDevice = false
	if err := e.Apply(s); err != nil {
		t.Fatal(err)
	}
	e.considerAutoSwitch(policy, s.Recording, "default", switchDevices, .001, now.Add(10*time.Second))
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 1 || !reflect.DeepEqual(capture.probes, []string{"default", "default", "default"}) || e.Status().AutoSwitchEnabled {
		t.Fatalf("unreliable or disabled switching still selected a device: %+v %+v", capture.options, capture.probes)
	}
}

func TestAutoSwitchDiscardsDecisionWhenUserHasDisabledSwitching(t *testing.T) {
	e, capture := autoSwitchEngine(t, map[string]float64{"default": .01, "usb": .9})
	previous := e.Settings().Recording
	s := e.Settings()
	s.Recording.AutoSwitchDevice = false
	if err := e.Apply(s); err != nil {
		t.Fatal(err)
	}
	policy, now := &audio.SwitchPolicy{}, time.Now()
	for i := range 3 {
		e.considerAutoSwitch(policy, previous, "default", switchDevices, .001, now.Add(time.Duration(i)*3*time.Second))
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.options) != 1 {
		t.Fatal("a stale probe overrode the user's disabled preference")
	}
}
