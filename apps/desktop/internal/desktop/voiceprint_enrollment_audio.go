package desktop

import (
	"context"
	"errors"
	"math"
	"sync/atomic"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

const (
	enrollmentAmbientMax    = 2 * audio.SampleRate
	enrollmentAmbientMin    = 2 * audio.SampleRate / 5
	enrollmentResumeSilence = 3 * audio.SampleRate / 10
	enrollmentPlaybackDB    = -50.0
)

const (
	voiceprintMsgAmbient       = "Stay quiet for a moment while the room is measured."
	voiceprintMsgPlayback      = "Playback was detected. Enrollment is paused and this audio will not be saved."
	voiceprintMsgReference     = "System playback cannot be checked. Pause videos yourself. Recording will continue."
	voiceprintErrNoise         = "The room was not quiet long enough to measure noise. Stay quiet and try again."
	voiceprintErrSNR           = "Background noise is too high. Move to a quieter place and record again."
	voiceprintErrInvalid       = "The recording contained invalid audio. Please try again."
	voiceprintErrDevice        = "The microphone or echo cancellation changed. Record again with the same input."
	voiceprintErrDeviceUnknown = "The selected microphone could not be identified. Choose an input device and try again."
	voiceprintErrInconsistent  = "These samples do not sound like the same person. Record all three samples again."
	voiceprintErrLoud          = "The recording is too loud. Move away from the microphone and try again."
	voiceprintErrMismatch      = "The verification voice did not match. Record your voice again in a quiet place."
	voiceprintErrNoSpeech      = "Not enough speech was detected. Check your microphone and try again."
)

type enrollmentTake struct {
	samples []float32
	device  string
	echo    bool
	noise   float64
}

// screenEnrollment is the last check before any embedding. Non-finite audio never passes.
func screenEnrollment(take enrollmentTake) error {
	switch voiceprint.AssessEnrollment(take.samples, take.noise) {
	case voiceprint.QualityInvalid:
		return errors.New(voiceprintErrInvalid)
	case voiceprint.QualityLowSNR:
		return errors.New(voiceprintErrSNR)
	}
	if voiceprint.Clipped(take.samples) {
		return errors.New(voiceprintErrLoud)
	}
	if take.device == "" || take.device == "default" {
		return errors.New(voiceprintErrDeviceUnknown)
	}
	return nil
}

// The session goroutine owns capture and VAD. Callbacks only enqueue PCM.
func (v *voiceprintEnrollment) collect(parent context.Context, step int, target float64, echo bool) (enrollmentTake, error) {
	ctx, cancel := context.WithTimeout(parent, v.timeout)
	defer cancel()
	vad, err := v.newVAD()
	if err != nil {
		return enrollmentTake{}, err
	}
	defer vad.Close()
	capture, err := v.newCapture()
	if err != nil {
		return enrollmentTake{}, err
	}
	defer capture.Close()
	packets, failures := make(chan audio.Packet, 64), make(chan error, 1)
	fail := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	var reference atomic.Bool
	v.showEnrollment(step, target, 0, 0, voiceprintMsgAmbient)
	err = capture.Start(audio.CaptureOptions{
		DeviceID: v.s.store.Settings().Recording.Device, EchoCancellation: echo, SystemReference: true,
		OnReferenceUnavailable: func(error) { reference.Store(true) },
	}, func(packet audio.Packet) {
		select {
		case packets <- packet:
		default:
			fail(errors.New("Voiceprint audio buffer overflow. Please try again."))
		}
	}, fail)
	if err != nil {
		return enrollmentTake{}, err
	}
	defer capture.Stop()
	state := enrollmentCollector{target: target, echo: echo}
	for {
		state.reference = reference.Load()
		if state.reference && state.paused {
			state.resume(vad)
			v.showEnrollment(step, target, state.seconds(), state.level, state.message())
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
				return enrollmentTake{}, errors.New(voiceprintErrNoSpeech)
			}
			return enrollmentTake{}, ctx.Err()
		case err := <-failures:
			return enrollmentTake{}, err
		case packet := <-packets:
			done, err := state.push(packet, vad)
			v.showEnrollment(step, target, state.seconds(), state.level, state.message())
			if err != nil {
				return enrollmentTake{}, err
			}
			if done {
				_ = capture.Stop()
				state.take.echo = echo
				state.take.noise = voiceprint.NoisePower(state.powers)
				if err := screenEnrollment(state.take); err != nil {
					return enrollmentTake{}, err
				}
				return state.take, nil
			}
		}
	}
}

func (v *voiceprintEnrollment) showEnrollment(step int, target, speech, level float64, message string) {
	v.update(func(s *protocol.VoiceprintSnapshot) {
		s.Step, s.TargetSeconds, s.SpeechSeconds, s.Level = step, target, math.Min(speech, target), math.Min(1, level)
		s.Phase, s.Message = protocol.VoiceprintRecording, message
	})
}

type enrollmentCollector struct {
	take      enrollmentTake
	pending   []float32
	powers    []float64
	examined  int
	noiseN    int
	voiced    int
	quiet     int
	target    float64
	level     float64
	paused    bool
	measured  bool
	reference bool
	echo      bool
}

func (c *enrollmentCollector) message() string {
	switch {
	case c.paused:
		return voiceprintMsgPlayback
	case !c.measured:
		return voiceprintMsgAmbient
	case c.reference:
		return voiceprintMsgReference
	default:
		return ""
	}
}

func (c *enrollmentCollector) seconds() float64 {
	if !c.measured {
		return 0
	}
	return float64(c.voiced) / audio.SampleRate
}

func (c *enrollmentCollector) push(packet audio.Packet, vad asr.VAD) (bool, error) {
	if !finitePacket(packet.Samples) {
		return false, errors.New(voiceprintErrInvalid)
	}
	if protocol.NormalizeAudioSource(packet.Source) == protocol.SourceSystem {
		c.system(packet.Samples, vad)
		return false, nil
	}
	if c.paused {
		return false, nil
	}
	device, err := enrollmentDevice(c.take.device, packet.DeviceID)
	if err != nil {
		return false, err
	}
	c.take.device = device
	c.level = audio.RMS(packet.Samples) * 10
	c.pending = append(c.pending, packet.Samples...)
	for len(c.pending) >= audio.WindowSize {
		window := append([]float32(nil), c.pending[:audio.WindowSize]...)
		c.pending = c.pending[audio.WindowSize:]
		if done, err := c.window(window, vad); err != nil || done {
			return done, err
		}
	}
	return false, nil
}

func (c *enrollmentCollector) window(window []float32, vad asr.VAD) (bool, error) {
	if !c.measured {
		return false, c.measure(window, vad)
	}
	vad.Accept(window)
	if vad.Detected() {
		c.voiced += audio.WindowSize
	}
	if err := c.drain(vad); err != nil {
		return false, err
	}
	if float64(c.voiced)/audio.SampleRate < c.target {
		return false, nil
	}
	vad.Flush()
	if err := c.drain(vad); err != nil {
		return false, err
	}
	if len(c.take.samples) < int(c.target*audio.SampleRate) {
		// Detected() includes VAD hangover that may be trimmed from returned
		// segments. Keep recording until the popped audio is long enough.
		c.voiced = len(c.take.samples)
		vad.Reset()
		return false, nil
	}
	return true, nil
}

func (c *enrollmentCollector) measure(window []float32, vad asr.VAD) error {
	c.examined += len(window)
	vad.Accept(window)
	if !vad.Detected() {
		c.powers = append(c.powers, voiceprint.MeanSquare(window))
		c.noiseN += len(window)
	}
	discardSpeech(vad)
	if c.noiseN < enrollmentAmbientMin && c.examined < enrollmentAmbientMax {
		return nil
	}
	if c.noiseN < enrollmentAmbientMin {
		return errors.New(voiceprintErrNoise)
	}
	c.measured = true
	c.pending = nil
	vad.Reset()
	return nil
}

func (c *enrollmentCollector) system(samples []float32, vad asr.VAD) {
	if c.reference {
		if c.paused {
			c.resume(vad)
		}
		return
	}
	if audio.Decibels(audio.RMS(samples)) > enrollmentPlaybackDB {
		c.pause(vad)
		return
	}
	if !c.paused {
		return
	}
	c.quiet += len(samples)
	if c.quiet >= enrollmentResumeSilence {
		c.resume(vad)
	}
}

func (c *enrollmentCollector) pause(vad asr.VAD) {
	c.paused = true
	c.quiet = 0
	c.pending = nil
	vad.Reset()
	c.voiced = len(c.take.samples)
}

func (c *enrollmentCollector) resume(vad asr.VAD) {
	c.paused = false
	c.quiet = 0
	c.pending = nil
	vad.Reset()
	c.voiced = len(c.take.samples)
}

func (c *enrollmentCollector) drain(vad asr.VAD) error {
	for {
		speech := vad.Pop()
		if len(speech.Samples) == 0 {
			return nil
		}
		if !finitePacket(speech.Samples) {
			return errors.New(voiceprintErrInvalid)
		}
		c.take.samples = append(c.take.samples, speech.Samples...)
	}
}

func discardSpeech(vad asr.VAD) {
	for len(vad.Pop().Samples) > 0 {
	}
}

func enrollmentDevice(have, next string) (string, error) {
	if next == "" || next == "default" {
		return "", errors.New(voiceprintErrDeviceUnknown)
	}
	if have != "" && have != next {
		return "", errors.New(voiceprintErrDevice)
	}
	return next, nil
}

func finitePacket(samples []float32) bool {
	for _, sample := range samples {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return false
		}
	}
	return true
}
