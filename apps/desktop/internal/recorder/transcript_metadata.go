package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func readTranscriptChunks(raw json.RawMessage, text string) []protocol.TranscriptChunk {
	chunks := []protocol.TranscriptChunk{}
	if json.Unmarshal(raw, &chunks) != nil || len(chunks) == 0 {
		return []protocol.TranscriptChunk{}
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		start, startErr := time.Parse(time.RFC3339Nano, chunk.StartedAt)
		end, endErr := time.Parse(time.RFC3339Nano, chunk.EndedAt)
		if startErr != nil || endErr != nil || end.Before(start) || strings.TrimSpace(chunk.Text) == "" {
			return []protocol.TranscriptChunk{}
		}
		texts[i] = chunk.Text
	}
	// Reprocessed or externally edited full text must never be replaced by
	// stale chunk text. Old metadata safely falls back to its recording time.
	if strings.TrimSpace(strings.Join(texts, "\n")) != text {
		return []protocol.TranscriptChunk{}
	}
	return chunks
}

func readCapturedRanges(raw json.RawMessage) []protocol.RecordingTimeRange {
	var ranges []protocol.RecordingTimeRange
	if json.Unmarshal(raw, &ranges) != nil {
		return nil
	}
	for _, capture := range ranges {
		start, startErr := time.Parse(time.RFC3339Nano, capture.StartedAt)
		end, endErr := time.Parse(time.RFC3339Nano, capture.EndedAt)
		if startErr != nil || endErr != nil || !end.After(start) {
			return nil
		}
	}
	return ranges
}

func batchMetadata(path, jsonPath, text, language string, model protocol.ASRModel) map[string]any {
	previous := map[string]any{}
	for _, file := range []string{strings.TrimSuffix(path, filepath.Ext(path)) + ".recording.json", jsonPath} {
		if raw, err := os.ReadFile(file); err == nil {
			_ = json.Unmarshal(raw, &previous)
		}
	}
	metadata := map[string]any{
		"audio_file": filepath.Base(path), "audio_path": path, "backend": "sherpa-onnx",
		"created_at": time.Now().Format(time.RFC3339), "model": asrmodel.Name(model), "status": "ok",
		"text": text, "language": language, "audio_source": protocol.SourceMicrophone, "segment_start_time": historyCandidate(path).StartedAt,
	}
	for _, key := range []string{"segment_start_time", "recording_session_id", "input_device", "audio_source"} {
		if value, ok := previous[key].(string); ok && value != "" {
			metadata[key] = value
		}
	}
	return metadata
}
