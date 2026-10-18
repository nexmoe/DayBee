package desktop

import (
	"context"
	"errors"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

func (v *voiceprintEnrollment) session(ctx context.Context, advance <-chan struct{}) error {
	if err := v.s.engine.Stop(); err != nil {
		return err
	}
	if v.s.engine.Busy() {
		return errors.New("Wait for the current transcription to finish before enrolling.")
	}
	if err := v.prepare(ctx); err != nil {
		return err
	}
	var groups [][][]float32
	var device string
	var echo bool
	bound := false
	for step := range voiceprintSamples + 1 {
		echoNow := v.s.store.Settings().Recording.EchoCancellation
		if bound && echoNow != echo {
			return errors.New(voiceprintErrDevice)
		}
		target := voiceprintSampleSeconds
		if step == voiceprintSamples {
			target = voiceprintVerifySeconds
		}
		take, err := v.collect(ctx, step, target, echoNow)
		if err != nil {
			return err
		}
		if err := screenEnrollment(take); err != nil {
			return err
		}
		if !bound {
			device, echo, bound = take.device, take.echo, true
		} else if take.device != device || take.echo != echo {
			return errors.New(voiceprintErrDevice)
		}
		v.update(func(s *protocol.VoiceprintSnapshot) { s.Phase = protocol.VoiceprintProcessing })
		if err := ctx.Err(); err != nil {
			return err
		}
		if step == voiceprintSamples {
			return v.finish(ctx, groups, take.samples, voiceprint.EnrollmentContext{Device: device, EchoCancellation: echo})
		}
		windows, _, err := v.profiles.EmbedWindows(take.samples, audio.SampleRate)
		if err != nil {
			return err
		}
		if len(windows) == 0 {
			return errors.New(voiceprintErrNoSpeech)
		}
		groups = append(groups, windows)
		if len(groups) == voiceprintSamples && voiceprint.InconsistentEnrollment(groups) {
			return errors.New(voiceprintErrInconsistent)
		}
		v.update(func(s *protocol.VoiceprintSnapshot) {
			s.Phase = protocol.VoiceprintSampleReady
			if s.Message != voiceprintMsgReference {
				s.Message = ""
			}
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-advance:
		}
	}
	return nil
}

func (v *voiceprintEnrollment) finish(ctx context.Context, groups [][][]float32, samples []float32, enctx voiceprint.EnrollmentContext) error {
	if voiceprint.InconsistentEnrollment(groups) {
		return errors.New(voiceprintErrInconsistent)
	}
	profile, err := voiceprint.Calibrate(groups, enctx)
	if err != nil {
		return err
	}
	scored, err := v.profiles.Score(profile, samples, audio.SampleRate)
	if err != nil {
		return err
	}
	identity, _ := voiceprint.DecideProfile(profile, voiceprint.DecisionScores(profile, scored), nil)
	if identity != "me" {
		return errors.New(voiceprintErrMismatch)
	}
	return v.commit(ctx, profile)
}

func (v *voiceprintEnrollment) commit(ctx context.Context, profile *voiceprint.Profile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous := v.profiles.Snapshot()
	previousSettings := v.s.store.Settings()
	if err := v.profiles.SaveProfile(profile); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return v.rollbackEnrollment(err, previous, previousSettings)
	}
	settings := previousSettings
	settings.Desktop.VoiceprintSetup = protocol.VoiceprintConfigured
	if err := v.save(settings); err != nil {
		return v.rollbackEnrollment(err, previous, previousSettings)
	}
	if err := ctx.Err(); err != nil {
		return v.rollbackEnrollment(err, previous, previousSettings)
	}
	return nil
}

func (v *voiceprintEnrollment) rollbackEnrollment(cause error, previous *voiceprint.Profile, previousSettings protocol.AppSettings) error {
	var restoreErr error
	if previous == nil {
		restoreErr = v.profiles.Clear()
	} else if err := v.profiles.SaveProfile(previous); err != nil {
		restoreErr = err
	}
	if v.s.store.Settings() != previousSettings {
		if err := v.save(previousSettings); err != nil {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	if restoreErr == nil {
		return cause
	}
	v.mu.Lock()
	v.rollbackFailed = true
	v.mu.Unlock()
	return errors.Join(cause, restoreErr)
}
