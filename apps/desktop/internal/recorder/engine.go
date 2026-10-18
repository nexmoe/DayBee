package recorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type Assets interface {
	SelectASRModel(protocol.ASRModel, ...string)
	Ensure(context.Context, bool, bool) error
	EnsureASRModel(context.Context, protocol.ASRModel, ...string) error
	VADPath() string
	ASRPath() string
	ASRModelPath(protocol.ASRModel, ...string) string
	Ready() (bool, bool)
	Transcode(context.Context, string, string) error
	ReadAudio(context.Context, string) ([]float32, int, error)
}

type command struct {
	run    func() error
	result chan error
}

// All capture/VAD/segment mutations belong to loop. Public operations are
// serialized independently, so waiting for ASR never blocks the audio consumer.
type Engine struct {
	ctx             context.Context
	cancel          context.CancelFunc
	op              sync.Mutex
	mu              sync.RWMutex
	settings        protocol.AppSettings
	status          protocol.RecorderStatusSnapshot
	devices         []protocol.DeviceInfo
	activeDevice    string
	prepareCancel   context.CancelFunc
	capture         audio.Capture
	assets          Assets
	runtime         asr.Runtime
	worker          *asr.Worker
	voice           voiceMatcher
	voiceAnchor     voiceAnchor
	echoRef         *echoReference
	commands        chan command
	packets         chan audio.Packet
	failures        chan error
	done            chan struct{}
	changed         func()
	historyChanged  chan struct{}
	waveform        func(protocol.WaveformFrame)
	waveformEnabled atomic.Bool
	finalizers      sync.WaitGroup
	finishing       atomic.Int64
	halts           atomic.Int64
	batch           atomic.Bool
	batchDone       chan struct{}
	batchCancel     context.CancelFunc
	refEpoch        atomic.Uint64
	refActive       atomic.Uint64
	refFailedEpoch  atomic.Uint64
	refPhase        atomic.Value
	active          sync.Map
	intent          bool
	suspended       bool
	closed          bool
	recoveryAt      time.Time
	recoveryDelay   time.Duration
	// The remaining fields are owned by loop.
	audioStream
	system     *audioStream
	spectrum   *audio.Spectrum
	started    time.Time
	refClock   time.Time
	now        func() time.Time
	voiceReady bool
	voiceCtx   voiceprint.EnrollmentContext
	sequence   uint64
	waveSilent bool
}

func New(settings protocol.AppSettings, capture audio.Capture, assets Assets, runtime asr.Runtime,
	changed func(), waveform func(protocol.WaveformFrame)) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{audioStream: audioStream{source: protocol.SourceMicrophone}, ctx: ctx, cancel: cancel, settings: settings, capture: capture, assets: assets,
		runtime: runtime, status: protocol.DefaultStatus(), devices: []protocol.DeviceInfo{},
		commands: make(chan command), packets: make(chan audio.Packet, 64), failures: make(chan error, 1),
		done: make(chan struct{}), changed: changed, historyChanged: make(chan struct{}, 1), waveform: waveform, spectrum: audio.NewSpectrum()}
	e.status.ASREnabled = !settings.Recording.DisableASR
	e.waveformEnabled.Store(true)
	assets.SelectASRModel(settings.Recording.ASRModel, settings.Recording.ASRLanguage)
	e.status.AutoSwitchEnabled = settings.Recording.AutoSwitchDevice
	e.status.VADReady, e.status.SenseVoiceReady = assets.Ready()
	e.worker = asr.NewModelWorker(ctx, assets.ASRPath, runtime.NewRecognizer)
	e.echoRef = newEchoReference()
	e.refPhase.Store(protocol.VoiceprintReferenceOff)
	go e.loop()
	go e.pollDevices()
	return e
}

// SetVoiceprint installs the process-wide speaker profile after New.
// The engine never closes it; shutdown closes the manager after Engine.Close.
func (e *Engine) SetVoiceprint(manager *voiceprint.Manager) {
	e.mu.Lock()
	if manager == nil {
		e.voice = nil
	} else {
		e.voice = manager
	}
	e.mu.Unlock()
	_ = e.call(func() error {
		e.rearmVoiceRuntime()
		return e.syncReferenceCapture()
	})
}

func (e *Engine) notify() {
	if e.changed != nil {
		e.changed()
	}
}

// SetWaveformEnabled pauses visualization work when no window can display it.
// Audio capture, VAD, metering and transcription continue independently.
func (e *Engine) SetWaveformEnabled(enabled bool) { e.waveformEnabled.Store(enabled) }

func (e *Engine) HistoryChanges() <-chan struct{} { return e.historyChanged }

func (e *Engine) historyUpdated() {
	select {
	case e.historyChanged <- struct{}{}:
	default:
	}
}
func (e *Engine) patch(fn func(*protocol.RecorderStatusSnapshot)) {
	e.mu.Lock()
	fn(&e.status)
	e.mu.Unlock()
	e.notify()
}
func (e *Engine) Status() protocol.RecorderStatusSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.status
	s.ASRHistory = append([]string{}, s.ASRHistory...)
	s.WaveformBins = append([]float64{}, s.WaveformBins...)
	return s
}
func (e *Engine) Devices() []protocol.DeviceInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]protocol.DeviceInfo{}, e.devices...)
}
func (e *Engine) Settings() protocol.AppSettings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.settings
}
func (e *Engine) Busy() bool {
	status := e.Status()
	return status.Recording || status.Recovering || e.batch.Load() || e.finishing.Load() > 0 || e.worker.Busy()
}
func (e *Engine) call(fn func() error) error {
	c := command{fn, make(chan error, 1)}
	select {
	case e.commands <- c:
	case <-e.ctx.Done():
		return context.Canceled
	}
	select {
	case err := <-c.result:
		return err
	case <-e.ctx.Done():
		return context.Canceled
	}
}
func (e *Engine) fail(err error) {
	if err == nil {
		return
	}
	slog.Error("Recorder operation failed", "error", err)
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		message := err.Error()
		s.Error = &message
		s.StatusMessage = message
	})
}

func (e *Engine) Start() error {
	e.op.Lock()
	defer e.op.Unlock()
	if e.closed || e.halts.Load() > 0 {
		return errors.New("Recorder is closed")
	}
	e.intent = true
	e.recoveryAt, e.recoveryDelay = time.Time{}, 0
	if e.suspended || e.Status().Recording {
		return nil
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) { s.Recovering = false })
	s := e.Settings()
	if err := e.prepare(true, !s.Recording.DisableASR); err != nil {
		e.intent = false
		e.fail(err)
		return err
	}
	// Playback still in the ring belongs to ASR callbacks from the previous take.
	e.finalizers.Wait()
	err := e.call(func() error { return e.start(s) })
	if err != nil {
		e.intent = false
		e.fail(err)
	}
	return err
}

func (e *Engine) Stop() error {
	e.halts.Add(1)
	defer e.halts.Add(-1)
	e.cancelPreparation()
	e.op.Lock()
	defer e.op.Unlock()
	e.intent = false
	e.recoveryAt, e.recoveryDelay = time.Time{}, 0
	return e.stopAndWait()
}
func (e *Engine) stopAndWait() error {
	defer e.worker.ReleaseWhenIdle()
	err := e.call(e.stop)
	e.finalizers.Wait()
	e.resetEcho()
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.Recording = false
		s.Recovering = false
		s.InSpeech = false
		s.Elapsed = "00:00:00"
		if s.Error == nil {
			s.StatusMessage = "Recording stopped."
		}
	})
	if err != nil {
		e.fail(err)
	}
	return err
}

func (e *Engine) Apply(settings protocol.AppSettings) error {
	e.op.Lock()
	defer e.op.Unlock()
	if e.closed {
		return context.Canceled
	}
	old := e.Settings()
	modelChanged := old.Recording.ASRModel != settings.Recording.ASRModel ||
		(settings.Recording.ASRModel == protocol.ASRSpeechAnalyzer && old.Recording.ASRLanguage != settings.Recording.ASRLanguage)
	if modelChanged && !e.Status().Recording && e.Busy() {
		return errors.New("Wait for transcription to finish before switching ASR models.")
	}
	if modelChanged && settings.Recording.ASRModel == protocol.ASRSpeechAnalyzer && !settings.Recording.DisableASR {
		if err := e.assets.EnsureASRModel(e.ctx, settings.Recording.ASRModel, settings.Recording.ASRLanguage); err != nil {
			return err
		}
	}
	if e.Status().Recording && !settings.Recording.DisableASR && old.Recording.DisableASR {
		if err := e.prepare(true, true); err != nil {
			return err
		}
	}
	err := e.call(func() error {
		// A failed rotation or device reopen must not leave a running status
		// with no segment/capture. The caller rolls persisted settings back.
		completed := false
		defer func() {
			if !completed {
				_ = e.stop()
				e.assets.SelectASRModel(old.Recording.ASRModel, old.Recording.ASRLanguage)
				e.mu.Lock()
				e.settings = old
				e.status.Recording = false
				e.status.InSpeech = false
				e.mu.Unlock()
			}
		}()
		deviceChanged := old.Recording.Device != settings.Recording.Device || old.Recording.EchoCancellation != settings.Recording.EchoCancellation || old.Recording.RecordSystemAudio != settings.Recording.RecordSystemAudio
		rotate := modelChanged || old.Recording.OutputDir != settings.Recording.OutputDir || old.Recording.DisableASR != settings.Recording.DisableASR || deviceChanged
		if e.vad != nil && deviceChanged {
			if err := e.capture.Stop(); err != nil {
				return err
			}
			for {
				select {
				case packet := <-e.packets:
					if err := e.consume(packet); err != nil {
						return err
					}
				default:
					goto drained
				}
			}
		}
	drained:
		if e.vad != nil && rotate {
			if err := e.rotate(); err != nil {
				return err
			}
		}
		oldWant := e.referenceWanted(old)
		e.mu.Lock()
		e.settings = settings
		e.mu.Unlock()
		if modelChanged {
			e.worker.ReleaseWhenIdle()
			e.assets.SelectASRModel(settings.Recording.ASRModel, settings.Recording.ASRLanguage)
		}
		if old.Recording.Device != settings.Recording.Device || old.Recording.EchoCancellation != settings.Recording.EchoCancellation {
			e.forceVoiceRuntime()
		}
		if e.vad != nil && (old.Recording.Device != settings.Recording.Device || old.Recording.EchoCancellation != settings.Recording.EchoCancellation) {
			e.resetEcho()
			voiceAnchorFor(e).clear()
			e.refClock = time.Time{}
			e.publishMismatch(e.activeDeviceID())
		}
		if e.vad != nil && rotate {
			if err := e.configureSystemAudio(settings); err != nil {
				return err
			}
			e.newSegment()
		}
		referenceStale := e.refActive.Load() != 0 && !e.wantsSystemReference()
		if e.vad != nil && deviceChanged {
			if err := e.startCapture(settings.Recording.Device, true); err != nil {
				return err
			}
		} else if e.vad != nil && (oldWant != e.wantsSystemReference() || referenceStale) {
			if err := e.restartCaptureOnly(settings.Recording.Device); err != nil {
				return err
			}
		}
		completed = true
		return nil
	})
	if err != nil {
		e.intent = false
		e.finalizers.Wait()
		e.fail(err)
		return err
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.VADReady, s.SenseVoiceReady = e.assets.Ready()
		s.ASREnabled = !settings.Recording.DisableASR
		s.AutoSwitchEnabled = settings.Recording.AutoSwitchDevice
		if settings.Recording.DisableASR {
			s.ASRPreview = ""
			s.ASRHistory = []string{}
		}
	})
	if settings.Recording.DisableASR {
		e.worker.ReleaseWhenIdle()
	}
	return nil
}

func (e *Engine) Suspend() error {
	e.halts.Add(1)
	defer e.halts.Add(-1)
	e.cancelPreparation()
	e.op.Lock()
	defer e.op.Unlock()
	e.suspended = true
	err := e.stopAndWait()
	if e.intent {
		e.patch(func(s *protocol.RecorderStatusSnapshot) {
			s.Recovering = true
			s.StatusMessage = "Recording paused while the computer sleeps."
		})
	}
	return err
}
func (e *Engine) Resume() error {
	e.op.Lock()
	defer e.op.Unlock()
	e.suspended = false
	// Wake and unlock both retry an interrupted recording. Keep the operation
	// locked so a concurrent manual Stop cannot be undone by a delayed Start.
	return e.recoverLocked(time.Now())
}
func (e *Engine) Close() error {
	e.halts.Add(1)
	defer e.halts.Add(-1)
	e.cancelPreparation()
	e.CancelBatch()
	e.op.Lock()
	defer e.op.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	e.intent = false
	e.CancelBatch()
	err := e.stopAndWait()
	e.mu.RLock()
	batchDone := e.batchDone
	e.mu.RUnlock()
	if batchDone != nil {
		<-batchDone
	}
	e.worker.Close()
	e.cancel()
	<-e.done
	return errors.Join(err, e.capture.Close())
}

func (e *Engine) prepare(vad, asr bool) error {
	ctx, cancel := context.WithCancel(e.ctx)
	e.mu.Lock()
	e.prepareCancel = cancel
	if e.halts.Load() > 0 {
		cancel()
	}
	e.mu.Unlock()
	defer func() { cancel(); e.mu.Lock(); e.prepareCancel = nil; e.mu.Unlock() }()
	return e.assets.Ensure(ctx, vad, asr)
}
func (e *Engine) cancelPreparation() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.prepareCancel != nil {
		e.prepareCancel()
	}
}

func elapsed(start time.Time) string {
	s := int(time.Since(start).Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}
