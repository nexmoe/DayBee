package voiceprint

import (
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
)

func perfMeasureClip(t *testing.T, manager *Manager, recognizer asr.Recognizer, clip perfClip) perfClipResult {
	t.Helper()
	seconds := float64(len(clip.Samples)) / voiceSampleRate
	result := perfClipResult{Name: clip.Name, Reference: clip.Reference, InputSeconds: seconds}
	result.Modes = append(result.Modes, perfRepeat(t, manager, recognizer, clip, seconds, "asr-only", false))
	result.Modes = append(result.Modes, perfRepeat(t, manager, recognizer, clip, seconds, "utterance-adaptation-off", false))
	result.Modes = append(result.Modes, perfRepeat(t, manager, recognizer, clip, seconds, "utterance-adaptation-on", true))
	return result
}

func perfRepeat(t *testing.T, manager *Manager, recognizer asr.Recognizer, clip perfClip, seconds float64, mode string, adapt bool) perfMode {
	t.Helper()
	out := perfMode{Mode: mode, WarmupExcluded: true}
	perfReset(t, manager, adapt)
	if warm := perfOnce(recognizer, manager, clip, seconds, mode != "asr-only"); warm.Error != "" {
		t.Fatalf("%s %s warmup: %s", clip.Name, mode, warm.Error)
	}
	for range 3 {
		perfReset(t, manager, adapt)
		out.Runs = append(out.Runs, perfOnce(recognizer, manager, clip, seconds, mode != "asr-only"))
	}
	voice, decode, total := make([]float64, len(out.Runs)), make([]float64, len(out.Runs)), make([]float64, len(out.Runs))
	maxAdapt := 0
	for i, run := range out.Runs {
		voice[i], decode[i], total[i] = run.VoiceprintMs, run.DecodeMs, run.TotalMs
		if run.AdaptationCount > maxAdapt {
			maxAdapt = run.AdaptationCount
		}
	}
	out.VoiceprintMeanMs, _, _ = perfSummary(voice)
	out.DecodeMeanMs, _, _ = perfSummary(decode)
	out.TotalMeanMs, out.TotalP50Ms, out.TotalP95Ms = perfSummary(total)
	if adapt && maxAdapt == 0 {
		out.Note = "adaptation stayed at 0; the conservative gate did not trigger, so this run supports no benefit conclusion"
	}
	return out
}

func perfReset(t *testing.T, manager *Manager, adapt bool) {
	t.Helper()
	manager.SetAdaptationEnabled(adapt)
	if err := manager.ClearAdaptation(); err != nil {
		t.Fatalf("clear adaptation before measurement: %v", err)
	}
	manager.ResetRuntime(runtimeEvalContext)
}

func perfOnce(recognizer asr.Recognizer, manager *Manager, clip perfClip, seconds float64, spans bool) perfRun {
	if !spans {
		started := time.Now()
		_, err := recognizer.Decode(clip.Samples, voiceSampleRate)
		elapsed := perfMillis(started)
		run := perfRun{DecodeCount: 1, DecodeMs: elapsed, TotalMs: elapsed, RTF: elapsed / 1000 / seconds, SpanCount: 1, CoverageComplete: err == nil}
		if err != nil {
			run.Error = err.Error()
		}
		return run
	}
	started := time.Now()
	parts := manager.MatchSpansContext(manager.Snapshot(), clip.Samples, voiceSampleRate, clip.Ref, runtimeEvalContext, false)
	run := perfRun{VoiceprintMs: perfMillis(started), SpanCount: len(parts)}
	if err := runtimeEvalCoverage(parts, len(clip.Samples)); err != nil {
		run.Error = err.Error()
		run.AdaptationCount, run.SystemCount = manager.RuntimeCounts()
		run.TotalMs = run.VoiceprintMs
		return run
	}
	run.CoverageComplete = true
	// Confidence spans describe matching evidence, not ASR input boundaries.
	part := time.Now()
	_, err := recognizer.Decode(clip.Samples, voiceSampleRate)
	run.DecodeMs = perfMillis(part)
	run.DecodeCount = 1
	if err != nil {
		run.Error = err.Error()
		run.CoverageComplete = false
	}
	run.AdaptationCount, run.SystemCount = manager.RuntimeCounts()
	run.TotalMs = run.VoiceprintMs + run.DecodeMs
	run.RTF = run.TotalMs / 1000 / seconds
	return run
}

func perfMillis(started time.Time) float64 {
	return float64(time.Since(started).Microseconds()) / 1000
}
