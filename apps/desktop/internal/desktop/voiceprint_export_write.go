package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type voiceprintExportFile struct {
	Path             string
	Name             string
	Source           protocol.AudioSource
	SessionID        string
	StartedAt        string
	Device           string
	EchoCancellation *bool
	CapturedRanges   []protocol.RecordingTimeRange
}

type voiceprintSampleFile struct {
	File             string                        `json:"file"`
	Source           protocol.AudioSource          `json:"source"`
	SessionID        string                        `json:"sessionId,omitempty"`
	StartedAt        string                        `json:"startedAt,omitempty"`
	Device           string                        `json:"device,omitempty"`
	EchoCancellation *bool                         `json:"echoCancellation,omitempty"`
	CapturedRanges   []protocol.RecordingTimeRange `json:"capturedRanges,omitempty"`
}

type voiceprintIdentityRange struct {
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
	Label     string `json:"label"`
}

type voiceprintSampleSidecar struct {
	Files          []voiceprintSampleFile    `json:"files"`
	IdentityRanges []voiceprintIdentityRange `json:"identityRanges"`
}

type captureMeta struct {
	CaptureDeviceID string `json:"capture_device_id"`
	Echo            *bool  `json:"echo_cancellation"`
}

// Tests replace this to simulate a failed copy.
var copyVoiceprintFile = copyRegularAudio

func voiceprintSampleDirectory(parent string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	for n := range 1000 {
		path := filepath.Join(parent, fmt.Sprintf("voiceprint-sample-%s-%d", stamp, n))
		err := os.Mkdir(path, 0700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return path, nil
	}
	return "", errors.New("Could not create a new voiceprint sample directory.")
}

func writeVoiceprintSample(dir string, files []voiceprintExportFile) error {
	sidecar := voiceprintSampleSidecar{Files: make([]voiceprintSampleFile, 0, len(files)), IdentityRanges: []voiceprintIdentityRange{}}
	for _, file := range files {
		if err := copyVoiceprintFile(file.Path, filepath.Join(dir, file.Name)); err != nil {
			return err
		}
		sidecar.Files = append(sidecar.Files, voiceprintSampleFile{
			File: file.Name, Source: file.Source, SessionID: file.SessionID, StartedAt: file.StartedAt,
			Device: file.Device, EchoCancellation: file.EchoCancellation, CapturedRanges: file.CapturedRanges,
		})
	}
	return config.WriteJSON(filepath.Join(dir, "samples.json"), sidecar)
}

func copyRegularAudio(src, dst string) error {
	before, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return errors.New("Voiceprint export only accepts regular WAV or FLAC files.")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	opened, err := in.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return errors.New("Voiceprint export only accepts regular WAV or FLAC files.")
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	return closeErr
}

func readCaptureMeta(audioPath string) captureMeta {
	base := strings.TrimSuffix(audioPath, filepath.Ext(audioPath))
	var meta captureMeta
	for _, path := range []string{base + ".json", base + ".recording.json"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var next captureMeta
		if json.Unmarshal(raw, &next) != nil {
			continue
		}
		if next.CaptureDeviceID != "" {
			meta.CaptureDeviceID = next.CaptureDeviceID
		}
		if next.Echo != nil {
			meta.Echo = next.Echo
		}
	}
	return meta
}
