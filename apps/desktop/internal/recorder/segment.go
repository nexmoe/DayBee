package recorder

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

type segment struct {
	id               uint64
	source           protocol.AudioSource
	start            time.Time
	sessionID        string
	settings         protocol.RecordingSettings
	modelPath        string
	device           string
	captureDeviceID  string
	echoCancellation bool
	captureContext   bool
	writer           *audio.WavWriter
	path             string
	jobs             sync.WaitGroup
	mu               sync.Mutex
	texts            []string
	chunks           []protocol.TranscriptChunk
	capturedRanges   []protocol.RecordingTimeRange
	diagnostics      []string
	sidecar          *recordingSidecar
}

func (e *Engine) newSegment() {
	for _, stream := range e.streams() {
		e.newStreamSegment(stream)
	}
}

func (e *Engine) newStreamSegment(stream *audioStream) {
	e.sequence++
	stream.currentID.Store(e.sequence)
	stream.segment = &segment{source: stream.source, id: e.sequence, start: time.Now(), sessionID: e.started.Format(time.RFC3339Nano),
		settings: e.Settings().Recording, modelPath: e.assets.ASRPath(), texts: []string{}, chunks: []protocol.TranscriptChunk{}}
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.ASRPreview = ""
		s.StatusMessage = "Started a new recording segment."
	})
}

func (e *Engine) appendSpeech(stream *audioStream, speech asr.Speech) error {
	seg := stream.segment
	samples := speech.Samples
	// VAD offsets include silence; archive offsets don't. Freeze the capture
	// times before submitting ASR so slow inference cannot move them forward.
	start := seg.start.Add(time.Duration(speech.Start) * time.Second / audio.SampleRate)
	end := start.Add(time.Duration(len(samples)) * time.Second / audio.SampleRate)
	if seg.writer == nil {
		dir := filepath.Join(seg.settings.OutputDir, seg.start.Format("20060102"))
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		base := "daybee_" + seg.start.Format("20060102_150405") + "_" + string(seg.source)
		seg.path = filepath.Join(dir, base+".wav")
		_, wavErr := os.Stat(seg.path)
		_, flacErr := os.Stat(filepath.Join(dir, base+".flac"))
		_, jsonErr := os.Stat(filepath.Join(dir, base+".json"))
		if wavErr == nil || flacErr == nil || jsonErr == nil {
			seg.path = filepath.Join(dir, fmt.Sprintf("%s_%d.wav", base, seg.start.UnixNano()))
		}
		writer, err := audio.NewWavWriter(seg.path)
		if err != nil {
			return err
		}
		seg.writer = writer
		e.active.Store(seg.path, true)
	}
	if err := seg.writer.Append(samples); err != nil {
		return err
	}
	source := stream.source
	var deviceID string
	var profile *voiceprint.Profile
	var matcher voiceMatcher
	if source == protocol.SourceMicrophone {
		e.mu.RLock()
		deviceID = e.activeDevice
		matcher = e.voice
		e.mu.RUnlock()
		if matcher != nil && matcher.Enrolled() {
			profile = matcher.Snapshot()
		}
	}
	seg.mu.Lock()
	if source == protocol.SourceMicrophone && !seg.captureContext {
		seg.captureDeviceID = deviceID
		seg.echoCancellation = seg.settings.EchoCancellation
		seg.captureContext = true
	}
	seg.capturedRanges = append(seg.capturedRanges, protocol.RecordingTimeRange{
		StartedAt: start.Format(time.RFC3339Nano), EndedAt: end.Format(time.RFC3339Nano),
	})
	explicit := seg.captureContext
	seg.mu.Unlock()
	if seg.sidecar == nil {
		seg.sidecar = newRecordingSidecar(seg)
	}
	seg.sidecar.queue(seg.path)
	if seg.settings.DisableASR {
		e.historyUpdated()
		return nil
	}
	// Profile, device, echo setting, and the system reference are frozen
	// before the queue wait. Completion must not read the echo ring.
	var ref *voiceprint.Reference
	if source == protocol.SourceMicrophone {
		ref = e.liveReference(start, end)
	} else {
		matcher = nil
		profile = nil
	}
	work := speechWork{stream: stream, seg: seg, samples: samples, start: start, end: end, profile: profile, matcher: matcher, ref: ref,
		context:  voiceprint.EnrollmentContext{Device: deviceID, EchoCancellation: seg.settings.EchoCancellation},
		explicit: explicit, session: seg.sessionID, device: deviceID}
	if !e.submitTranscript(work) {
		seg.addDiagnostic("Live ASR queue full; speech was archived.")
		slog.Warn("Live ASR queue full; speech was archived", "audio", seg.path)
	}
	return nil
}

func (e *Engine) finishSegment(stream *audioStream) error {
	seg := stream.segment
	stream.segment = nil
	if seg == nil || seg.writer == nil {
		return nil
	}
	err := seg.writer.Close()
	if err != nil {
		seg.mu.Lock()
		seg.diagnostics = append(seg.diagnostics, err.Error())
		seg.mu.Unlock()
	}
	if seg.sidecar != nil {
		err = errors.Join(err, seg.sidecar.finish(seg.path))
	}
	e.finalizers.Add(1)
	e.finishing.Add(1)
	go func() {
		defer e.finalizers.Done()
		defer e.finishing.Add(-1)
		defer e.active.Delete(seg.path)
		defer e.notify()
		defer e.historyUpdated()
		seg.jobs.Wait()
		if finalizeErr := e.archive(seg); finalizeErr != nil {
			e.fail(finalizeErr)
		}
	}()
	return err
}

func (e *Engine) archive(seg *segment) error {
	path := seg.path
	if seg.settings.AudioFormat == protocol.FLAC {
		flac := strings.TrimSuffix(path, ".wav") + ".flac"
		if err := e.assets.Transcode(e.ctx, path, flac); err != nil {
			slog.Error("FLAC conversion failed; preserving WAV", "audio", path, "error", err)
			seg.diagnostics = append(seg.diagnostics, err.Error())
		} else {
			path = flac
			if err := os.Remove(seg.path); err != nil {
				slog.Warn("Unable to remove converted WAV", "error", err)
			}
		}
	}
	if err := writeRecordingMetadata(seg, path); err != nil {
		return err
	}
	if seg.settings.DisableASR {
		return nil
	}
	return writeSegmentMetadata(seg, path)
}

// Keep capture ranges separately from ASR results. Pure recording and later
// re-transcription must retain their original scene timing, including silence
// removed from VAD-only audio. Never infer capture time from file duration.
func writeRecordingMetadata(seg *segment, path string) error {
	seg.mu.Lock()
	defer seg.mu.Unlock()
	metadata := map[string]any{
		"audio_path": path, "segment_start_time": seg.start.Format(time.RFC3339Nano),
		"recording_session_id": seg.sessionID, "captured_ranges": seg.capturedRanges,
		"input_device": seg.device, "audio_source": seg.source,
	}
	if seg.captureContext {
		metadata["capture_device_id"] = seg.captureDeviceID
		metadata["echo_cancellation"] = seg.echoCancellation
	}
	return config.WriteJSON(strings.TrimSuffix(path, filepath.Ext(path))+".recording.json", metadata)
}

func writeSegmentMetadata(seg *segment, path string) error {
	seg.mu.Lock()
	defer seg.mu.Unlock()
	metadata := map[string]any{
		"audio_file": filepath.Base(path), "audio_path": path, "backend": "sherpa-onnx",
		"created_at": time.Now().Format(time.RFC3339), "audio_source": seg.source, "input_device": seg.device, "model": asrmodel.Name(seg.settings.ASRModel),
		"segment_start_time": seg.start.Format(time.RFC3339), "status": "ok", "text": strings.Join(seg.texts, "\n"),
		"recording_session_id": seg.sessionID, "transcript_chunks": seg.chunks,
	}
	if len(seg.diagnostics) > 0 {
		metadata["transcription_incomplete"] = true
		metadata["diagnostics"] = seg.diagnostics
	}
	return config.WriteJSON(strings.TrimSuffix(path, filepath.Ext(path))+".json", metadata)
}
