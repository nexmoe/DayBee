package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/recorder"
)

func (s *DesktopService) ExportVoiceprintSamples(recordingID, destinationDir string) (string, error) {
	if err := s.guard(); err != nil {
		return "", err
	}
	if s.engine == nil || s.engine.Busy() {
		return "", errors.New("Finish recording and transcription before exporting voiceprint samples.")
	}
	parent, err := voiceprintExportParent(destinationDir)
	if err != nil {
		return "", err
	}
	history := recorder.History(s.voiceprintHistoryRoots())
	primary, ok := voiceprintHistoryByID(history, recordingID)
	if !ok {
		return "", errors.New("That recording is not in the current history.")
	}
	if primary.Status == "recording" {
		return "", errors.New("That recording is not finished.")
	}
	files, err := voiceprintSampleSelection(primary, history, s.voiceprintHistoryRoots())
	if err != nil {
		return "", err
	}
	dir, err := voiceprintSampleDirectory(parent)
	if err != nil {
		return "", err
	}
	if err := writeVoiceprintSample(dir, files); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func (s *DesktopService) voiceprintHistoryRoots() []string {
	var roots []string
	if s.store != nil {
		roots = append(roots, s.store.Settings().Recording.OutputDir)
	}
	if s.defaultOutput != "" {
		roots = append(roots, s.defaultOutput)
	}
	roots = append(roots, s.legacyOutputs...)
	return roots
}

func voiceprintExportParent(dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("Choose a folder for the voiceprint samples.")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Choose a folder for the voiceprint samples.")
	}
	return dir, nil
}

func voiceprintHistoryByID(history []protocol.RecordingHistoryItem, id string) (protocol.RecordingHistoryItem, bool) {
	for _, item := range history {
		if item.ID == id && item.AudioPath != "" {
			return item, true
		}
	}
	return protocol.RecordingHistoryItem{}, false
}

func voiceprintSampleSelection(primary protocol.RecordingHistoryItem, history []protocol.RecordingHistoryItem, roots []string) ([]voiceprintExportFile, error) {
	selected := []protocol.RecordingHistoryItem{primary}
	for _, item := range history {
		if item.ID == primary.ID || item.AudioPath == primary.AudioPath || item.Status == "recording" || !voiceprintSibling(primary, item) {
			continue
		}
		selected = append(selected, item)
	}
	files := make([]voiceprintExportFile, 0, len(selected))
	for _, item := range selected {
		file, err := voiceprintCheckedFile(item, roots)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func voiceprintCheckedFile(item protocol.RecordingHistoryItem, roots []string) (voiceprintExportFile, error) {
	if err := regularAudioFile(item.AudioPath); err != nil {
		return voiceprintExportFile{}, err
	}
	if !voiceprintInsideRoots(item.AudioPath, roots) {
		return voiceprintExportFile{}, errors.New("That recording is not in the current history.")
	}
	meta := readCaptureMeta(item.AudioPath)
	device := meta.CaptureDeviceID
	if device == "" && item.InputDevice != nil {
		device = *item.InputDevice
	}
	return voiceprintExportFile{
		Path: item.AudioPath, Name: filepath.Base(item.AudioPath), Source: item.AudioSource,
		SessionID: item.SessionID, StartedAt: item.StartedAt, Device: device,
		EchoCancellation: meta.Echo, CapturedRanges: append([]protocol.RecordingTimeRange{}, item.CapturedRanges...),
	}, nil
}

func voiceprintInsideRoots(path string, roots []string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || !filepath.IsAbs(resolved) {
		return false
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		rootResolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rootResolved, err = filepath.Abs(rootResolved)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootResolved, resolved)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}

func voiceprintSibling(primary, item protocol.RecordingHistoryItem) bool {
	if primary.SessionID == "" || item.SessionID != primary.SessionID || !voiceprintCounterpart(primary.AudioSource, item.AudioSource) {
		return false
	}
	if stamp, ok := voiceprintSegmentStamp(primary.AudioPath); ok {
		if other, otherOK := voiceprintSegmentStamp(item.AudioPath); otherOK && other == stamp {
			return true
		}
	}
	return voiceprintRangesOverlap(primary.CapturedRanges, item.CapturedRanges)
}

func voiceprintCounterpart(primary, other protocol.AudioSource) bool {
	if primary == protocol.SourceSystem {
		return other != protocol.SourceSystem
	}
	return other == protocol.SourceSystem
}

func voiceprintSegmentStamp(path string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "daybee_")
	if !ok {
		return "", false
	}
	for _, source := range []string{string(protocol.SourceMicrophone), string(protocol.SourceSystem)} {
		index := strings.Index(rest, "_"+source)
		if index != len("20060102_150405") || !voiceprintClockStamp(rest[:index]) {
			continue
		}
		tail := rest[index+len(source)+1:]
		if tail == "" || (len(tail) > 1 && tail[0] == '_' && digitsOnly(tail[1:])) {
			return rest[:index], true
		}
	}
	return "", false
}

func voiceprintClockStamp(stamp string) bool {
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

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func voiceprintRangesOverlap(left, right []protocol.RecordingTimeRange) bool {
	for _, a := range left {
		start, end, ok := voiceprintRange(a)
		if !ok {
			continue
		}
		for _, b := range right {
			otherStart, otherEnd, ok := voiceprintRange(b)
			if !ok {
				continue
			}
			if start.Before(otherEnd) && otherStart.Before(end) {
				return true
			}
		}
	}
	return false
}

func voiceprintRange(span protocol.RecordingTimeRange) (time.Time, time.Time, bool) {
	start, startErr := time.Parse(time.RFC3339Nano, span.StartedAt)
	end, endErr := time.Parse(time.RFC3339Nano, span.EndedAt)
	return start, end, startErr == nil && endErr == nil && start.Before(end)
}

func regularAudioFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (ext != ".wav" && ext != ".flac") {
		return errors.New("Voiceprint export only accepts regular WAV or FLAC files.")
	}
	return nil
}
