package desktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/assets"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

const (
	voiceprintSamples       = 3
	voiceprintSampleSeconds = 8.0
	voiceprintVerifySeconds = 4.0
)

type enrollmentProfiles interface {
	Enrolled() bool
	Snapshot() *voiceprint.Profile
	EmbedWindows([]float32, int) ([][]float32, []int, error)
	Score(*voiceprint.Profile, []float32, int) ([]voiceprint.WindowScore, error)
	Model() string
	SaveProfile(*voiceprint.Profile) error
	Clear() error
}

type enrollmentCapture interface {
	Start(audio.CaptureOptions, func(audio.Packet), func(error)) error
	Stop() error
	Close() error
}

type voiceprintEnrollment struct {
	mu             sync.Mutex
	s              *DesktopService
	profiles       enrollmentProfiles
	prepare        func(context.Context) error
	newCapture     func() (enrollmentCapture, error)
	newVAD         func() (asr.VAD, error)
	snapshot       protocol.VoiceprintSnapshot
	active         bool
	rollbackFailed bool
	closed         bool
	cancel         context.CancelFunc
	done           chan struct{}
	advance        chan struct{}
	timeout        time.Duration
	save           func(protocol.AppSettings) error
}

func newVoiceprintEnrollment(s *DesktopService, models *assets.Manager, manager *voiceprint.Manager) voiceprintSetup {
	v := &voiceprintEnrollment{s: s, profiles: manager, snapshot: protocol.DefaultVoiceprint(), timeout: time.Minute}
	v.prepare = func(ctx context.Context) error {
		if err := models.Ensure(ctx, true, false); err != nil {
			return err
		}
		return models.EnsureVoiceprint(ctx)
	}
	v.newCapture = func() (enrollmentCapture, error) { return audio.NewNativeCapture() }
	v.newVAD = func() (asr.VAD, error) { return (asr.Sherpa{}).NewVAD(models.VADPath()) }
	v.save = func(settings protocol.AppSettings) error { _, err := s.SaveSettings(settings); return err }
	return v
}

func (v *voiceprintEnrollment) Snapshot() protocol.VoiceprintSnapshot {
	v.mu.Lock()
	snapshot := v.snapshot
	failed := v.rollbackFailed
	v.mu.Unlock()
	snapshot.Enrolled = v.profiles != nil && v.profiles.Enrolled() && !failed
	return snapshot
}

func (v *voiceprintEnrollment) Active() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active
}

func (v *voiceprintEnrollment) update(fn func(*protocol.VoiceprintSnapshot)) {
	v.mu.Lock()
	fn(&v.snapshot)
	v.mu.Unlock()
	v.s.markDirty()
}

func (v *voiceprintEnrollment) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.active {
		return errors.New("Voiceprint enrollment is already active or closed.")
	}
	ctx, cancel := context.WithCancel(v.s.ctx)
	v.cancel, v.done, v.advance = cancel, make(chan struct{}), make(chan struct{}, 1)
	v.active = true
	v.rollbackFailed = false
	v.snapshot = protocol.DefaultVoiceprint()
	v.snapshot.Phase = protocol.VoiceprintPreparing
	status := v.s.engine.Status()
	resume := status.Recording || status.Recovering
	first := v.s.store.Settings().Desktop.VoiceprintSetup == protocol.VoiceprintPending
	go v.run(ctx, v.done, v.advance, resume, first)
	return nil
}

func (v *voiceprintEnrollment) Advance() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active || v.snapshot.Phase != protocol.VoiceprintSampleReady {
		return errors.New("Wait until enough speech has been recorded.")
	}
	v.snapshot.Phase = protocol.VoiceprintPreparing
	v.advance <- struct{}{}
	v.s.markDirty()
	return nil
}

func (v *voiceprintEnrollment) Cancel() {
	v.mu.Lock()
	cancel, done := v.cancel, v.done
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	v.mu.Lock()
	if v.done == done && v.snapshot.Phase != protocol.VoiceprintReady {
		v.snapshot = protocol.DefaultVoiceprint()
	}
	v.mu.Unlock()
	v.s.markDirty()
}

func (v *voiceprintEnrollment) Close() {
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()
	v.Cancel()
}

func (v *voiceprintEnrollment) Clear() error {
	v.Cancel()
	if err := v.profiles.Clear(); err != nil {
		return err
	}
	v.update(func(s *protocol.VoiceprintSnapshot) { *s = protocol.DefaultVoiceprint() })
	return nil
}

func (v *voiceprintEnrollment) run(ctx context.Context, done chan struct{}, advance <-chan struct{}, resume, first bool) {
	err := v.session(ctx, advance)
	v.mu.Lock()
	closed := v.closed
	rollbackFailed := v.rollbackFailed
	if err != nil {
		if canceledOnly(err) {
			v.snapshot = protocol.DefaultVoiceprint()
		} else {
			message := err.Error()
			v.snapshot.Phase, v.snapshot.Error = protocol.VoiceprintError, &message
			v.snapshot.Message, v.snapshot.Level, v.snapshot.Progress = "", 0, nil
		}
	} else {
		v.snapshot.Phase = protocol.VoiceprintReady
	}
	v.mu.Unlock()
	v.s.markDirty()
	if !closed && !v.s.quitting.Load() && v.s.ctx.Err() == nil {
		if err == nil {
			requested := v.s.voiceStartRequested.Swap(false)
			resume = resume || requested || (first && !v.s.test && v.s.store.Settings().Desktop.StartRecordingOnLaunch)
		}
		if resume && !rollbackFailed {
			_ = v.s.engine.Start()
		}
	}
	v.mu.Lock()
	v.active = false
	v.mu.Unlock()
	close(done)
}

func canceledOnly(err error) bool {
	if err == nil || !errors.Is(err, context.Canceled) {
		return false
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return true
	}
	for _, part := range joined.Unwrap() {
		if !canceledOnly(part) {
			return false
		}
	}
	return true
}
