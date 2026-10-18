package desktop

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type enrollmentTestProfiles struct {
	*voiceprint.Manager
	matched     bool
	outlier     int
	failRestore bool
	failClear   bool
	saved       atomic.Int32
	embedded    atomic.Int32
	checked     atomic.Int32
	mu          sync.Mutex
	lens        []int
}

func (p *enrollmentTestProfiles) EmbedWindows(samples []float32, _ int) ([][]float32, []int, error) {
	call := int(p.embedded.Add(1))
	p.mu.Lock()
	p.lens = append(p.lens, len(samples))
	p.mu.Unlock()
	offset := float32(call-1) * 0.01
	vectors := make([][]float32, 4)
	starts := make([]int, len(vectors))
	for i := range vectors {
		vectors[i] = make([]float32, 192)
		vectors[i][0] = 1
		vectors[i][1] = offset + float32(i)*0.0001
		starts[i] = i
		if p.outlier == call {
			vectors[i][0], vectors[i][1], vectors[i][2] = 0, 0, 1
		}
	}
	return vectors, starts, nil
}

func (p *enrollmentTestProfiles) lengths() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.lens...)
}

func (p *enrollmentTestProfiles) Score(profile *voiceprint.Profile, samples []float32, _ int) ([]voiceprint.WindowScore, error) {
	if profile == nil || profile.Version != 3 || profile.Model != voiceprint.ModelName || profile.Calibration == nil || profile.Device == "" || profile.Device == "default" {
		return nil, errors.New("enrollment scored an incomplete profile")
	}
	p.checked.Store(int32(len(samples)))
	vector := make([]float32, 192)
	vector[0] = 1
	vector[1] = 0.01
	if !p.matched {
		vector[0], vector[1], vector[2] = 0, 0, 1
	}
	windows := make([]voiceprint.WindowScore, 8)
	for i := range windows {
		embedding := append([]float32(nil), vector...)
		windows[i] = voiceprint.WindowScore{Score: voiceprint.ProfileScore(profile, embedding), Embedding: embedding}
	}
	return windows, nil
}

func (p *enrollmentTestProfiles) SaveProfile(profile *voiceprint.Profile) error {
	if p.failRestore && p.saved.Add(1) > 1 {
		return errors.New("voiceprint restore failed")
	}
	if !p.failRestore {
		p.saved.Add(1)
	}
	return p.Manager.SaveProfile(profile)
}

func (p *enrollmentTestProfiles) Clear() error {
	if p.failClear {
		return errors.New("voiceprint clear failed")
	}
	return p.Manager.Clear()
}

type enrollmentTestCapture struct {
	speech                 bool
	wave                   []float32
	device                 string
	script                 []audio.Packet
	useGate                bool
	gateAfter              int
	gate                   chan struct{}
	release                chan struct{}
	failReferenceAtStart   bool
	failReferenceAfterGate bool
	closed                 *atomic.Int32
	active                 *atomic.Int32
	stop                   chan struct{}
	done                   chan struct{}
	stopped                atomic.Bool
}

func (c *enrollmentTestCapture) Start(options audio.CaptureOptions, packet func(audio.Packet), fail func(error)) error {
	if options.SystemAudio || !options.SystemReference {
		return errors.New("enrollment must monitor playback without archiving it")
	}
	c.active.Add(1)
	c.stop, c.done = make(chan struct{}), make(chan struct{})
	device := c.device
	if device == "" {
		device = "native:enrollment-mic"
	}
	silenceWindows := (enrollmentAmbientMin + audio.WindowSize - 1) / audio.WindowSize
	go func() {
		defer close(c.done)
		if c.failReferenceAtStart && options.OnReferenceUnavailable != nil {
			options.OnReferenceUnavailable(errors.New("reference unavailable"))
		}
		if len(c.script) > 0 {
			c.playScript(options, packet, device)
			<-c.stop
			return
		}
		for n := range silenceWindows + 2000 {
			select {
			case <-c.stop:
				return
			default:
			}
			x := make([]float32, audio.WindowSize)
			if n >= silenceWindows {
				if c.speech {
					x[0] = 0.25
				}
				if len(c.wave) > 0 {
					for i := range x {
						x[i] = c.wave[((n-silenceWindows)*audio.WindowSize+i)%len(c.wave)]
					}
				}
			}
			packet(audio.Packet{Samples: x, DeviceID: device, Source: protocol.SourceMicrophone})
			time.Sleep(time.Millisecond)
		}
	}()
	return nil
}

func (c *enrollmentTestCapture) playScript(options audio.CaptureOptions, packet func(audio.Packet), device string) {
	for i, item := range c.script {
		select {
		case <-c.stop:
			return
		default:
		}
		if item.DeviceID == "" {
			item.DeviceID = device
		}
		if item.Source == "" {
			item.Source = protocol.SourceMicrophone
		}
		packet(item)
		time.Sleep(time.Millisecond)
		if !c.useGate || i != c.gateAfter {
			continue
		}
		if c.gate != nil {
			close(c.gate)
		}
		if c.release != nil {
			select {
			case <-c.release:
			case <-c.stop:
				return
			}
		}
		if c.failReferenceAfterGate && options.OnReferenceUnavailable != nil {
			options.OnReferenceUnavailable(errors.New("reference unavailable"))
		}
	}
}
func (c *enrollmentTestCapture) Stop() error {
	if c.stop != nil && c.stopped.CompareAndSwap(false, true) {
		close(c.stop)
		<-c.done
		c.active.Add(-1)
	}
	return nil
}
func (c *enrollmentTestCapture) Close() error { _ = c.Stop(); c.closed.Add(1); return nil }

type enrollmentTestVAD struct {
	speech  bool
	samples []float32
}

func (v *enrollmentTestVAD) Accept(x []float32) {
	v.speech = x[0] != 0
	if v.speech {
		v.samples = append(v.samples, x...)
	}
}
func (v *enrollmentTestVAD) Detected() bool { return v.speech }
func (v *enrollmentTestVAD) Pop() asr.Speech {
	x := v.samples
	v.samples = nil
	return asr.Speech{Samples: x}
}
func (v *enrollmentTestVAD) Flush() {}
func (v *enrollmentTestVAD) Reset() { v.samples, v.speech = nil, false }
func (v *enrollmentTestVAD) Close() {}

func enrollmentHarness(t *testing.T, speech, matched bool) (*voiceprintEnrollment, *enrollmentTestProfiles, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	store, err := config.Open(t.TempDir(), t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &DesktopService{ctx: ctx, store: store, test: true}
	s.engine = newTestEngine(store.Settings(), func() {})
	manager, _ := voiceprint.New(t.TempDir(), nil)
	profiles := &enrollmentTestProfiles{Manager: manager, matched: matched}
	closed, active := &atomic.Int32{}, &atomic.Int32{}
	v := &voiceprintEnrollment{s: s, profiles: profiles, snapshot: protocol.DefaultVoiceprint(), timeout: time.Second,
		prepare: func(context.Context) error { return nil },
		newCapture: func() (enrollmentCapture, error) {
			return &enrollmentTestCapture{speech: speech, closed: closed, active: active}, nil
		},
		newVAD: func() (asr.VAD, error) { return &enrollmentTestVAD{}, nil },
		save:   func(settings protocol.AppSettings) error { _, err := store.Save(settings); return err },
	}
	s.voiceSetup = v
	t.Cleanup(v.Close)
	return v, profiles, closed, active
}

func waitEnrollment(t *testing.T, v *voiceprintEnrollment, phase protocol.VoiceprintPhase) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := v.Snapshot()
		if snapshot.Phase == phase {
			return
		}
		if snapshot.Phase == protocol.VoiceprintError && snapshot.Error != nil {
			t.Fatalf("expected %s, enrollment failed: %s", phase, *snapshot.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %s, got %+v", phase, v.Snapshot())
}

func finishEnrollmentSamples(t *testing.T, v *voiceprintEnrollment) {
	t.Helper()
	if err := v.Start(); err != nil {
		t.Fatal(err)
	}
	for range voiceprintSamples {
		waitEnrollment(t, v, protocol.VoiceprintSampleReady)
		if err := v.Advance(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVoiceprintEnrollmentRequiresSpeechAndCancelsCleanly(t *testing.T) {
	v, profiles, closed, active := enrollmentHarness(t, false, true)
	v.timeout = 40 * time.Millisecond
	if err := v.Start(); err != nil {
		t.Fatal(err)
	}
	if err := v.Start(); err == nil {
		t.Fatal("duplicate enrollment created a session")
	}
	if err := v.Advance(); err == nil {
		t.Fatal("silence advanced enrollment")
	}
	waitEnrollment(t, v, protocol.VoiceprintError)
	v.Cancel()
	if profiles.saved.Load() != 0 || profiles.Enrolled() || closed.Load() != 1 || active.Load() != 0 {
		t.Fatal("silence saved a profile or retained capture")
	}
	if err := v.Start(); err != nil {
		t.Fatal(err)
	}
	v.Cancel()
	if v.Active() || profiles.saved.Load() != 0 || active.Load() != 0 {
		t.Fatal("cancel retained an active session")
	}
}

func TestVoiceprintEnrollmentVerificationAndRecordingRestore(t *testing.T) {
	for _, matched := range []bool{false, true} {
		t.Run(map[bool]string{false: "mismatch", true: "matched"}[matched], func(t *testing.T) {
			v, profiles, closed, active := enrollmentHarness(t, true, matched)
			_ = v.s.engine.Start()
			finishEnrollmentSamples(t, v)
			phase := protocol.VoiceprintError
			if matched {
				phase = protocol.VoiceprintReady
			}
			waitEnrollment(t, v, phase)
			if !matched && (v.Snapshot().Step != voiceprintSamples || enrollmentError(v) != voiceprintErrMismatch) {
				t.Fatalf("verification failure lost the step or error: step %d %q", v.Snapshot().Step, enrollmentError(v))
			}
			v.Cancel() // joins cleanup and restoration
			if !v.s.engine.Status().Recording || active.Load() != 0 || closed.Load() != voiceprintSamples+1 {
				t.Fatal("recording intent or capture cleanup was lost")
			}
			settings := v.s.store.Settings()
			if profiles.Enrolled() != matched || (settings.Desktop.VoiceprintSetup == protocol.VoiceprintConfigured) != matched {
				t.Fatal("verification did not govern activation")
			}
			if !matched && profiles.saved.Load() != 0 {
				t.Fatal("verification failure saved a profile")
			}
			if matched {
				got := profiles.Snapshot()
				if got == nil || got.Version != 3 || got.Device != "native:enrollment-mic" || got.Calibration == nil || !(got.Calibration.THigh > got.Calibration.TLow) {
					t.Fatalf("stored %+v", got)
				}
				if profiles.embedded.Load() != voiceprintSamples || profiles.checked.Load() < int32(voiceprintVerifySeconds*audio.SampleRate) {
					t.Fatalf("embedded %d, verify samples %d", profiles.embedded.Load(), profiles.checked.Load())
				}
				for _, n := range profiles.lengths() {
					if n < int(voiceprintSampleSeconds*audio.SampleRate) {
						t.Fatalf("registration segment was %d samples", n)
					}
				}
			}
		})
	}
}

func TestVoiceprintEnrollmentRejectsClipping(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, true)
	closed, active := &atomic.Int32{}, &atomic.Int32{}
	wave := make([]float32, audio.WindowSize)
	for i := range wave {
		wave[i] = 1
	}
	v.newCapture = func() (enrollmentCapture, error) {
		return &enrollmentTestCapture{wave: wave, closed: closed, active: active}, nil
	}
	if err := v.Start(); err != nil {
		t.Fatal(err)
	}
	waitEnrollment(t, v, protocol.VoiceprintError)
	message := ""
	if text := v.Snapshot().Error; text != nil {
		message = *text
	}
	v.Cancel()
	if message != "The recording is too loud. Move away from the microphone and try again." || profiles.embedded.Load() != 0 || profiles.saved.Load() != 0 {
		t.Fatalf("clipped speech was embedded or saved: %q", message)
	}
}

func TestVoiceprintEnrollmentFailedRerecordPreservesProfile(t *testing.T) {
	v, profiles, _, _ := enrollmentHarness(t, true, false)
	old := make([]float32, 192)
	old[1] = 1
	if err := profiles.Manager.Save([][]float32{old}); err != nil {
		t.Fatal(err)
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintError)
	v.Cancel()
	if profiles.Snapshot().Embeddings[0][1] != 1 || profiles.saved.Load() != 0 {
		t.Fatal("failed verification replaced the old voiceprint")
	}
}

func TestNativeVoiceprintEnrollment(t *testing.T) {
	model, vadPath, speech := os.Getenv("DAYBEE_TEST_VOICEPRINT_MODEL"), os.Getenv("DAYBEE_TEST_VAD_PATH"), os.Getenv("DAYBEE_TEST_VOICEPRINT_SPEECH")
	if model == "" || vadPath == "" || speech == "" {
		t.Skip("Set voiceprint model, Silero model and offline speech fixture")
	}
	wave := sherpa.ReadWave(speech)
	if wave == nil || wave.SampleRate != audio.SampleRate {
		t.Fatal("a 16 kHz speech fixture is required")
	}
	v, _, closed, active := enrollmentHarness(t, true, true)
	manager, err := voiceprint.New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	v.profiles = manager
	v.timeout = 5 * time.Second
	v.newVAD = func() (asr.VAD, error) { return (asr.Sherpa{}).NewVAD(vadPath) }
	v.newCapture = func() (enrollmentCapture, error) {
		return &enrollmentTestCapture{wave: wave.Samples, closed: closed, active: active}, nil
	}
	finishEnrollmentSamples(t, v)
	waitEnrollment(t, v, protocol.VoiceprintReady)
	v.Cancel()
	if !manager.Enrolled() || v.s.store.Settings().Desktop.VoiceprintSetup != protocol.VoiceprintConfigured || active.Load() != 0 {
		t.Fatal("native enrollment did not finish and release capture")
	}
}
