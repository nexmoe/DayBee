package recorder

import (
	"errors"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

var ErrPCMOverflow = errors.New("Recording audio buffer overflow. Recording was stopped safely.")

func (e *Engine) loop() {
	defer close(e.done)
	wave := time.NewTicker(30 * time.Millisecond)
	defer wave.Stop()
	clock := time.NewTicker(100 * time.Millisecond)
	defer clock.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case cmd := <-e.commands:
			cmd.result <- cmd.run()
		case packet := <-e.packets:
			if err := e.consume(packet); err != nil {
				e.safeStop(err)
			}
		case err := <-e.failures:
			if errors.Is(err, audio.ErrDisconnected) || errors.Is(err, audio.ErrInterrupted) {
				e.interrupted(err)
			} else {
				e.safeStop(err)
			}
		case <-wave.C:
			if e.vad == nil || e.waveform == nil || !e.waveformEnabled.Load() {
				continue
			}
			frame := e.spectrum.Frame()
			silent := spectrumSilent(frame)
			if silent && e.waveSilent {
				continue
			}
			e.waveSilent = silent
			e.waveform(protocol.WaveformFrame{Spectrum: frame})
		case <-clock.C:
			e.settleReferenceFailure()
			if e.vad == nil {
				continue
			}
			e.patch(func(s *protocol.RecorderStatusSnapshot) { s.Elapsed = elapsed(e.started) })
			if time.Since(e.segment.start) >= time.Duration(e.Settings().Recording.SegmentMinutes*float64(time.Minute)) {
				if err := e.rotate(); err != nil {
					e.safeStop(err)
				} else {
					e.newSegment()
				}
			}
		}
	}
}

func (e *Engine) safeStop(err error) {
	// Called only by loop. Stop capture before draining so the queue is finite.
	_ = e.stop()
	e.fail(err)
	var epoch uint64
	if e.echoRef != nil {
		epoch = e.echoRef.Epoch()
	}
	go func() {
		e.op.Lock()
		defer e.op.Unlock()
		e.intent = false
		e.finalizers.Wait()
		e.worker.ReleaseWhenIdle()
		if e.echoRef != nil {
			e.echoRef.ResetIf(epoch)
		}
		e.patch(func(s *protocol.RecorderStatusSnapshot) {
			s.Recording = false
			s.Recovering = false
			s.InSpeech = false
		})
	}()
}

func (e *Engine) start(settings protocol.AppSettings) error {
	if e.vad != nil {
		return nil
	}
	e.rearmVoiceRuntime()
	e.resetEcho()
	vad, err := e.runtime.NewVAD(e.assets.VADPath())
	if err != nil {
		return err
	}
	e.vad = vad
	e.remainder = nil
	e.spectrum = audio.NewSpectrum()
	e.started = time.Now()
	if err = e.configureSystemAudio(settings); err != nil {
		e.closeStreams()
		return err
	}
	e.newSegment()
	if err = e.startCapture(settings.Recording.Device, true); err != nil {
		e.closeStreams()
		return err
	}
	e.started = time.Now()
	for _, stream := range e.streams() {
		stream.segment.sessionID = e.started.Format(time.RFC3339Nano)
	}
	vr, ar := e.assets.Ready()
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.Error = nil
		s.Recording = true
		s.Recovering = false
		s.InSpeech = false
		s.ASRPreview = ""
		s.ASRHistory = []string{}
		s.Elapsed = "00:00:00"
		s.VADReady = vr
		s.SenseVoiceReady = ar
		s.StatusMessage = "Recording with " + asrmodel.Name(settings.Recording.ASRModel) + "."
		if settings.Recording.DisableASR {
			s.StatusMessage = "Recording audio only."
		}
	})
	return nil
}

func (e *Engine) startCapture(id string, resetClock bool) error {
	options := e.captureOptions(id)
	epoch := e.refEpoch.Load()
	err := e.capture.Start(options, func(packet audio.Packet) {
		packet.CaptureEpoch = epoch
		if packet.CapturedAt.IsZero() {
			packet.CapturedAt = time.Now()
		}
		select {
		case e.packets <- packet:
		default:
			select {
			case e.failures <- ErrPCMOverflow:
			default:
			}
		}
	}, func(err error) {
		select {
		case e.failures <- err:
		default:
		}
	})
	if err != nil {
		e.retireReference()
		return err
	}
	// Authorization and device startup may take seconds. Audio before both
	// sources are ready is gated by Capture, so their clocks start here.
	if resetClock {
		for _, stream := range e.streams() {
			stream.segment.start = time.Now()
		}
	}
	return nil
}

func (e *Engine) consume(packet audio.Packet) error {
	e.settleReferenceFailure()
	if packet.CaptureEpoch != 0 && packet.CaptureEpoch != e.refEpoch.Load() {
		return nil
	}
	stream := &e.audioStream
	if protocol.NormalizeAudioSource(packet.Source) == protocol.SourceSystem {
		e.feedSystemReference(packet)
		if e.system == nil || e.system.vad == nil {
			return nil
		}
		stream = e.system
	}
	if stream == nil || stream.vad == nil {
		return nil
	}
	if stream.source == protocol.SourceMicrophone {
		e.spectrum.Push(packet.Samples)
		rms := audio.RMS(packet.Samples)
		e.patch(func(s *protocol.RecorderStatusSnapshot) {
			s.RMS = rms
			s.DB = audio.Decibels(rms)
			s.LevelRatio = rms
			s.DeviceLabel = packet.DeviceLabel
			s.WaveformBins = audio.WaveformBins(packet.Samples)
		})
		e.noteActiveDevice(packet.DeviceID)
	}
	if stream.segment.device == "" {
		stream.segment.device = packet.DeviceLabel
	}
	stream.remainder = append(stream.remainder, packet.Samples...)
	for len(stream.remainder) >= audio.WindowSize {
		// Silero's Start offset counts every sample since Reset, including
		// silence. Skipping Accept on a quiet window would move later speech
		// earlier, so digital silence is still fed to the detector.
		stream.vad.Accept(stream.remainder[:audio.WindowSize])
		stream.remainder = stream.remainder[audio.WindowSize:]
		if err := e.drainVAD(stream); err != nil {
			return err
		}
	}
	speech := false
	for _, stream := range e.streams() {
		speech = speech || stream.vad.Detected()
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) { s.InSpeech = speech })
	return nil
}

func spectrumSilent(frame []int) bool {
	for _, bin := range frame {
		if bin != 0 {
			return false
		}
	}
	return true
}

func (e *Engine) drainVAD(stream *audioStream) error {
	for {
		speech := stream.vad.Pop()
		if len(speech.Samples) == 0 {
			return nil
		}
		if err := e.appendSpeech(stream, speech); err != nil {
			return err
		}
	}
}

func (e *Engine) rotate() error {
	var err error
	for _, stream := range e.streams() {
		err = errors.Join(err, e.rotateStream(stream))
	}
	return err
}

func (e *Engine) rotateStream(stream *audioStream) error {
	if stream.vad == nil {
		return nil
	}
	if len(stream.remainder) > 0 {
		padded := make([]float32, audio.WindowSize)
		copy(padded, stream.remainder)
		stream.vad.Accept(padded)
		stream.remainder = nil
	}
	stream.vad.Flush()
	err := e.drainVAD(stream)
	stream.vad.Reset()
	return errors.Join(err, e.finishSegment(stream))
}

func (e *Engine) stop() error {
	if e.vad == nil {
		e.retireReference()
		return nil
	}
	if e.segment == nil {
		e.newSegment()
	}
	err := e.capture.Stop()
	for {
		select {
		case packet := <-e.packets:
			err = errors.Join(err, e.consume(packet))
		default:
			goto drained
		}
	}
drained:
	err = errors.Join(err, e.rotate())
	e.retireReference()
	e.closeStreams()
	for {
		select {
		case <-e.failures:
		default:
			return err
		}
	}
}
