package recorder

import (
	"log/slog"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type speechWork struct {
	stream   *audioStream
	seg      *segment
	samples  []float32
	start    time.Time
	end      time.Time
	profile  *voiceprint.Profile
	matcher  voiceMatcher
	ref      *voiceprint.Reference
	context  voiceprint.EnrollmentContext
	explicit bool
	session  string
	device   string
}

func (e *Engine) submitTranscript(work speechWork) bool {
	work.seg.jobs.Add(1)
	var job asr.Job
	if _, ok := work.matcher.(spanMatcher); ok && work.profile != nil {
		job = e.spanJob(work)
	} else {
		job = e.plainJob(work)
	}
	if e.worker.SubmitLive(job) {
		return true
	}
	work.seg.jobs.Done()
	return false
}

func (e *Engine) plainJob(work speechWork) asr.Job {
	return asr.Job{Context: e.ctx, ModelPath: work.seg.modelPath,
		Prepare: func() ([]float32, int, error) { return work.samples, audio.SampleRate, nil },
		Complete: func(result asr.Result, err error) {
			defer work.seg.jobs.Done()
			identity, score := "", float32(0)
			if err == nil && result.Text != "" {
				identity, score = matchVoice(work.matcher, work.profile, work.samples, work.ref)
			}
			if err != nil {
				work.seg.addDiagnostic(err.Error())
				slog.Error("Speech archived without transcription", "audio", work.seg.path, "error", err)
				return
			}
			if result.Text != "" {
				e.saveTranscript(work.stream, work.seg, result.Text, work.start, work.end, identity, "", float64(score))
			}
		}}
}

func (e *Engine) spanJob(work speechWork) asr.Job {
	return asr.Job{Context: e.ctx, ModelPath: work.seg.modelPath,
		Spans: func(decode func([]float32, int) (asr.Result, error)) error {
			return e.decodeIdentity(decode, work)
		},
		Complete: func(_ asr.Result, err error) {
			defer work.seg.jobs.Done()
			if err != nil {
				work.seg.addDiagnostic(err.Error())
				slog.Error("Speech archived without transcription", "audio", work.seg.path, "error", err)
			}
		}}
}

func (e *Engine) decodeIdentity(decode func([]float32, int) (asr.Result, error), work speechWork) error {
	spans := e.identitySpans(work)
	allow := spanMeAllowed(work.profile, work.explicit, work.context)
	identity, match, score, seed := utteranceVerdict(spans, len(work.samples), allow)
	var err error
	wrote := false
	if len(work.samples) > 0 && e.ctx.Err() == nil {
		var result asr.Result
		result, err = decode(work.samples, audio.SampleRate)
		wrote = err == nil && result.Text != ""
		if wrote {
			e.saveTranscript(work.stream, work.seg, result.Text, work.start, work.end, identity, match, score)
		}
	}
	end := time.Time{}
	if wrote && seed {
		end = work.end
	}
	// A partial or unanchored utterance clears the next-VAD relaxation anchor.
	voiceAnchorFor(e).observe(work.session, work.device, end, work.ref, false, wrote && seed)
	return err
}

func (e *Engine) identitySpans(work speechWork) []voiceprint.IdentitySpan {
	n := len(work.samples)
	fallback := []voiceprint.IdentitySpan{{Start: 0, End: n, Identity: "uncertain"}}
	splitter, ok := work.matcher.(spanMatcher)
	if !ok || work.profile == nil || n == 0 {
		return fallback
	}
	relaxed := voiceAnchorFor(e).allow(work.session, work.device, work.start, work.ref)
	var spans []voiceprint.IdentitySpan
	func() {
		defer func() { _ = recover() }()
		if contextual, ok := work.matcher.(contextSpanMatcher); ok {
			spans = contextual.MatchSpansContext(work.profile, work.samples, audio.SampleRate, work.ref, work.context, relaxed)
			return
		}
		spans = splitter.MatchSpansRelaxed(work.profile, work.samples, audio.SampleRate, work.ref, relaxed)
	}()
	if !spansCover(spans, n) {
		return fallback
	}
	return spans
}

func spansCover(spans []voiceprint.IdentitySpan, n int) bool {
	if n <= 0 || len(spans) == 0 || spans[0].Start != 0 || spans[len(spans)-1].End != n {
		return false
	}
	for i, span := range spans {
		if span.End <= span.Start || (i > 0 && span.Start != spans[i-1].End) {
			return false
		}
	}
	return true
}

func atSample(start time.Time, offset, rate int) time.Time {
	if rate <= 0 || offset <= 0 {
		return start
	}
	return start.Add(time.Duration(offset) * time.Second / time.Duration(rate))
}

func (e *Engine) saveTranscript(stream *audioStream, seg *segment, text string, started, ended time.Time, identity, match string, score float64) {
	seg.mu.Lock()
	seg.texts = append(seg.texts, text)
	chunk := protocol.TranscriptChunk{Text: text, StartedAt: started.Format(time.RFC3339Nano), EndedAt: ended.Format(time.RFC3339Nano), VoiceScore: score, VoiceMatch: match}
	if identity == "me" {
		chunk.VoiceIdentity = "me"
	}
	seg.chunks = append(seg.chunks, chunk)
	seg.mu.Unlock()
	// A fast recognizer may finish before the asynchronous scene writer. Both
	// metadata files must be durable before notifying history readers.
	if writeErr := writeRecordingMetadata(seg, seg.path); writeErr != nil {
		e.fail(writeErr)
		return
	}
	if writeErr := writeSegmentMetadata(seg, seg.path); writeErr != nil {
		e.fail(writeErr)
		return
	}
	e.historyUpdated()
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		if stream == nil || stream.currentID.Load() != seg.id || !s.ASREnabled {
			return
		}
		if s.ASRPreview != "" {
			s.ASRHistory = append([]string{s.ASRPreview}, s.ASRHistory...)
		}
		s.ASRPreview = text
		if len(s.ASRHistory) > 5 {
			s.ASRHistory = s.ASRHistory[:5]
		}
		s.StatusMessage = "Speech recognized."
	})
}

func (seg *segment) addDiagnostic(text string) {
	seg.mu.Lock()
	seg.diagnostics = append(seg.diagnostics, text)
	seg.mu.Unlock()
}
