package recorder

import (
	"context"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

func captureContext(meta map[string]any) (voiceprint.EnrollmentContext, bool) {
	device, deviceOK := meta["capture_device_id"].(string)
	aec, aecOK := meta["echo_cancellation"].(bool)
	if !deviceOK || !aecOK {
		return voiceprint.EnrollmentContext{}, false
	}
	return voiceprint.EnrollmentContext{Device: device, EchoCancellation: aec}, true
}

func (e *Engine) labelSpeechPieces(ctx context.Context, matcher voiceMatcher, pieces []speechPiece, track *systemTrack, meta map[string]any, modelPath string) ([]string, []protocol.TranscriptChunk, string, error) {
	capture, explicit := captureContext(meta)
	session := jsonString(meta, "recording_session_id")
	anchor := &voiceAnchor{}
	texts := []string{}
	chunks := []protocol.TranscriptChunk{}
	language := ""
	for _, piece := range pieces {
		if err := ctx.Err(); err != nil {
			return nil, nil, "", err
		}
		profile := matcher.Snapshot()
		ref := track.reference(piece)
		start, _ := time.Parse(time.RFC3339Nano, piece.started)
		nextTexts, nextChunks, pieceLanguage, err := e.labelOnePiece(ctx, matcher, profile, piece, ref, capture, explicit, session, start, anchor, modelPath)
		if err != nil {
			e.fail(err)
			return nil, nil, "", err
		}
		if language == "" {
			language = pieceLanguage
		}
		texts = append(texts, nextTexts...)
		chunks = append(chunks, nextChunks...)
	}
	return texts, chunks, language, nil
}

func (e *Engine) labelOnePiece(ctx context.Context, matcher voiceMatcher, profile *voiceprint.Profile, piece speechPiece, ref *voiceprint.Reference, capture voiceprint.EnrollmentContext, explicit bool, session string, start time.Time, anchor *voiceAnchor, modelPath string) ([]string, []protocol.TranscriptChunk, string, error) {
	if splitter, ok := matcher.(spanMatcher); ok && profile != nil {
		return e.labelSpanPiece(ctx, splitter, profile, piece, ref, capture, explicit, session, start, anchor, modelPath)
	}
	result, err := e.decodeSamples(ctx, piece.samples, audio.SampleRate, modelPath)
	if err != nil || result.Text == "" {
		return nil, nil, result.Language, err
	}
	identity, score := matchVoice(matcher, profile, piece.samples, ref)
	chunk := protocol.TranscriptChunk{Text: result.Text, StartedAt: piece.started, EndedAt: piece.ended, VoiceScore: float64(score)}
	if identity == "me" {
		chunk.VoiceIdentity = "me"
	}
	return []string{result.Text}, []protocol.TranscriptChunk{chunk}, result.Language, nil
}

func (e *Engine) labelSpanPiece(ctx context.Context, splitter spanMatcher, profile *voiceprint.Profile, piece speechPiece, ref *voiceprint.Reference, capture voiceprint.EnrollmentContext, explicit bool, session string, start time.Time, anchor *voiceAnchor, modelPath string) ([]string, []protocol.TranscriptChunk, string, error) {
	relaxed := anchor.allow(session, capture.Device, start, ref)
	spans := guardedSpans(splitter, profile, piece.samples, ref, relaxed)
	if !spansCover(spans, len(piece.samples)) {
		spans = []voiceprint.IdentitySpan{{Start: 0, End: len(piece.samples), Identity: "uncertain"}}
	}
	if len(piece.samples) == 0 {
		anchor.observe(session, capture.Device, time.Time{}, ref, false, false)
		return nil, nil, "", nil
	}
	result, err := e.decodeSamples(ctx, piece.samples, audio.SampleRate, modelPath)
	if err != nil {
		return nil, nil, "", err
	}
	allow := spanMeAllowed(profile, explicit, capture)
	identity, match, score, seed := utteranceVerdict(spans, len(piece.samples), allow)
	if result.Text == "" {
		anchor.observe(session, capture.Device, time.Time{}, ref, false, false)
		return nil, nil, result.Language, nil
	}
	ended := atSample(start, len(piece.samples), audio.SampleRate)
	chunk := protocol.TranscriptChunk{
		Text: result.Text, StartedAt: start.Format(time.RFC3339Nano), EndedAt: ended.Format(time.RFC3339Nano),
		VoiceScore: score, VoiceMatch: match,
	}
	if identity == "me" {
		chunk.VoiceIdentity = "me"
	}
	meEnd := time.Time{}
	if seed {
		meEnd = ended
	}
	anchor.observe(session, capture.Device, meEnd, ref, false, seed)
	return []string{result.Text}, []protocol.TranscriptChunk{chunk}, result.Language, nil
}

func guardedSpans(splitter spanMatcher, profile *voiceprint.Profile, samples []float32, ref *voiceprint.Reference, relax bool) (spans []voiceprint.IdentitySpan) {
	defer func() { _ = recover() }()
	return splitter.MatchSpansRelaxed(profile, samples, audio.SampleRate, ref, relax)
}
