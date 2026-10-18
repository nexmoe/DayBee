package recorder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type speechPiece struct {
	samples []float32
	started string
	ended   string
}

// transcribeLabeled splits a microphone file into speech pieces when a saved
// voiceprint is available. Every piece is transcribed; matches are marked me.
// System audio and files without a usable profile keep the whole-file path.
func (e *Engine) transcribeLabeled(ctx context.Context, path, jsonPath string, model protocol.ASRModel, modelPath string) (bool, error) {
	e.mu.RLock()
	matcher := e.voice
	e.mu.RUnlock()
	if matcher == nil || !matcher.Enrolled() {
		return false, nil
	}
	meta := mergedMetadata(path, jsonPath)
	if protocol.NormalizeAudioSource(protocol.AudioSource(jsonString(meta, "audio_source"))) == protocol.SourceSystem {
		return false, nil
	}
	samples, rate, err := e.assets.ReadAudio(ctx, path)
	if err != nil {
		return false, err
	}
	// Silero and speaker matching both expect 16 kHz. Any other rate stays on
	// the normal whole-file path so labels are not applied to the wrong clock.
	if rate != audio.SampleRate {
		return false, nil
	}
	ranges := rangesFromMetadata(meta)
	pieces, ok := piecesFromRanges(samples, rate, ranges)
	if !ok {
		base := time.Now()
		if parsed, err := time.Parse(time.RFC3339Nano, jsonString(meta, "segment_start_time")); err == nil {
			base = parsed
		} else if started := historyCandidate(path).StartedAt; started != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, started); err == nil {
				base = parsed
			} else if parsed, err := time.ParseInLocation("2006-01-02T15:04:05", started, time.Local); err == nil {
				base = parsed
			}
		}
		vad, vadErr := e.runtime.NewVAD(e.assets.VADPath())
		if vadErr != nil {
			return false, nil
		}
		defer vad.Close()
		pieces = piecesFromVAD(vad, samples, rate, base)
	}
	if len(pieces) == 0 {
		return false, nil
	}
	track := e.systemTrack(ctx, path)
	texts, chunks, language, err := e.labelSpeechPieces(ctx, matcher, pieces, track, meta, modelPath)
	if err != nil {
		return false, err
	}
	metadata := batchMetadata(path, jsonPath, strings.Join(texts, "\n"), language, model)
	metadata["transcript_chunks"] = chunks
	if capture, explicit := captureContext(meta); explicit {
		metadata["capture_device_id"] = capture.Device
		metadata["echo_cancellation"] = capture.EchoCancellation
	}
	if len(ranges) > 0 {
		metadata["captured_ranges"] = ranges
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := config.WriteJSON(jsonPath, metadata); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Engine) decodeSamples(ctx context.Context, samples []float32, rate int, modelPath string) (asr.Result, error) {
	type completion struct {
		result asr.Result
		err    error
	}
	done := make(chan completion, 1)
	job := asr.Job{Context: ctx, ModelPath: modelPath, Prepare: func() ([]float32, int, error) {
		return samples, rate, nil
	}, Complete: func(result asr.Result, err error) { done <- completion{result, err} }}
	if err := e.worker.SubmitBatch(job); err != nil {
		return asr.Result{}, err
	}
	completed := <-done
	if err := ctx.Err(); err != nil {
		return asr.Result{}, err
	}
	return completed.result, completed.err
}

func mergedMetadata(path, jsonPath string) map[string]any {
	merged := map[string]any{}
	for _, file := range []string{strings.TrimSuffix(path, filepath.Ext(path)) + ".recording.json", jsonPath} {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(raw, &payload) == nil {
			for key, value := range payload {
				merged[key] = value
			}
		}
	}
	return merged
}

func jsonString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func rangesFromMetadata(metadata map[string]any) []protocol.RecordingTimeRange {
	raw, err := json.Marshal(metadata["captured_ranges"])
	if err != nil {
		return nil
	}
	return readCapturedRanges(raw)
}

func piecesFromRanges(samples []float32, rate int, ranges []protocol.RecordingTimeRange) ([]speechPiece, bool) {
	if len(ranges) == 0 || rate <= 0 {
		return nil, false
	}
	counts := make([]int, len(ranges))
	total := 0
	for i, item := range ranges {
		start, startErr := time.Parse(time.RFC3339Nano, item.StartedAt)
		end, endErr := time.Parse(time.RFC3339Nano, item.EndedAt)
		if startErr != nil || endErr != nil || !end.After(start) {
			return nil, false
		}
		count := int(end.Sub(start) * time.Duration(rate) / time.Second)
		if count <= 0 {
			return nil, false
		}
		counts[i] = count
		total += count
	}
	if total != len(samples) {
		if total < len(samples) && len(samples)-total < len(ranges) {
			counts[len(counts)-1] += len(samples) - total
			total = len(samples)
		}
	}
	if total != len(samples) {
		return nil, false
	}
	pieces := make([]speechPiece, len(ranges))
	offset := 0
	for i, item := range ranges {
		next := offset + counts[i]
		pieces[i] = speechPiece{samples: append([]float32{}, samples[offset:next]...), started: item.StartedAt, ended: item.EndedAt}
		offset = next
	}
	return pieces, true
}

func piecesFromVAD(vad asr.VAD, samples []float32, rate int, base time.Time) []speechPiece {
	pieces := []speechPiece{}
	take := func() {
		for {
			speech := vad.Pop()
			if len(speech.Samples) == 0 {
				return
			}
			start := base.Add(time.Duration(speech.Start) * time.Second / time.Duration(rate))
			end := start.Add(time.Duration(len(speech.Samples)) * time.Second / time.Duration(rate))
			pieces = append(pieces, speechPiece{samples: append([]float32{}, speech.Samples...), started: start.Format(time.RFC3339Nano), ended: end.Format(time.RFC3339Nano)})
		}
	}
	for len(samples) >= audio.WindowSize {
		vad.Accept(samples[:audio.WindowSize])
		samples = samples[audio.WindowSize:]
		take()
	}
	if len(samples) > 0 {
		padded := make([]float32, audio.WindowSize)
		copy(padded, samples)
		vad.Accept(padded)
		take()
	}
	vad.Flush()
	take()
	return pieces
}

// systemTrack is the sibling system archive for one microphone file.
// daybee_<YYYYMMDD_HHMMSS>_system.wav/.flac, including a numeric collision suffix.
// A missing file, another rate, or captured ranges that do not match the PCM is ignored.
type systemTrack struct {
	pieces []speechPiece
}

func (e *Engine) systemTrack(ctx context.Context, micPath string) *systemTrack {
	if e == nil || e.assets == nil {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(micPath), filepath.Ext(micPath))
	stamp, ok := segmentStamp(base, string(protocol.SourceMicrophone))
	if !ok {
		return nil
	}
	for _, path := range systemAudioFiles(filepath.Dir(micPath), stamp) {
		samples, rate, err := e.assets.ReadAudio(ctx, path)
		if err != nil || rate != audio.SampleRate || len(samples) == 0 {
			continue
		}
		pieces, aligned := piecesFromRanges(samples, rate, rangesFromMetadata(mergedMetadata(path, strings.TrimSuffix(path, filepath.Ext(path))+".json")))
		if !aligned {
			continue
		}
		return &systemTrack{pieces: pieces}
	}
	return nil
}

func (t *systemTrack) reference(piece speechPiece) *voiceprint.Reference {
	if t == nil {
		return nil
	}
	samples, complete := alignPieces(t.pieces, piece.started, piece.ended, len(piece.samples))
	if samples == nil {
		return nil
	}
	return &voiceprint.Reference{Samples: samples, Complete: complete}
}

func alignPieces(pieces []speechPiece, started, ended string, n int) ([]float32, bool) {
	if n <= 0 || len(pieces) == 0 {
		return nil, false
	}
	start, err1 := time.Parse(time.RFC3339Nano, started)
	end, err2 := time.Parse(time.RFC3339Nano, ended)
	if err1 != nil || err2 != nil || !end.After(start) {
		return nil, false
	}
	out := make([]float32, n)
	seen := make([]bool, n)
	filled := 0
	hit := false
	for _, piece := range pieces {
		ps, psErr := time.Parse(time.RFC3339Nano, piece.started)
		pe, peErr := time.Parse(time.RFC3339Nano, piece.ended)
		if psErr != nil || peErr != nil || !pe.After(ps) || len(piece.samples) == 0 {
			continue
		}
		lo, hi := start, end
		if ps.After(lo) {
			lo = ps
		}
		if pe.Before(hi) {
			hi = pe
		}
		if !hi.After(lo) {
			continue
		}
		dst, src := sampleOffset(start, lo), sampleOffset(ps, lo)
		count := sampleOffset(lo, hi)
		if dst < 0 || src < 0 || count <= 0 || dst >= n || src >= len(piece.samples) {
			continue
		}
		if dst+count > n {
			count = n - dst
		}
		if src+count > len(piece.samples) {
			count = len(piece.samples) - src
		}
		if count <= 0 {
			continue
		}
		copy(out[dst:dst+count], piece.samples[src:src+count])
		for i := 0; i < count; i++ {
			if !seen[dst+i] {
				seen[dst+i] = true
				filled++
			}
		}
		hit = true
	}
	if !hit {
		return nil, false
	}
	return out, filled == n
}

func sampleOffset(from, to time.Time) int {
	if to.Before(from) {
		return -1
	}
	return int(to.Sub(from) * time.Duration(audio.SampleRate) / time.Second)
}

func systemAudioFiles(dir, stamp string) []string {
	var paths []string
	for _, ext := range []string{".wav", ".flac"} {
		exact := filepath.Join(dir, "daybee_"+stamp+"_system"+ext)
		if regularFile(exact) {
			paths = append(paths, exact)
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "daybee_"+stamp+"_system_*"+ext))
		for _, match := range matches {
			name := strings.TrimSuffix(filepath.Base(match), ext)
			if _, ok := segmentStamp(name, string(protocol.SourceSystem)); ok && regularFile(match) {
				paths = append(paths, match)
			}
		}
	}
	return paths
}

func segmentStamp(base, source string) (string, bool) {
	rest, ok := strings.CutPrefix(base, "daybee_")
	marker := "_" + source
	if !ok {
		return "", false
	}
	index := strings.Index(rest, marker)
	if index != len("20060102_150405") || !clockStamp(rest[:index]) {
		return "", false
	}
	tail := rest[index+len(marker):]
	if tail == "" {
		return rest[:index], true
	}
	if len(tail) < 2 || tail[0] != '_' {
		return "", false
	}
	for _, c := range tail[1:] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return rest[:index], true
}

func clockStamp(stamp string) bool {
	if len(stamp) != 15 || stamp[8] != '_' {
		return false
	}
	for i, c := range stamp {
		if i != 8 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
