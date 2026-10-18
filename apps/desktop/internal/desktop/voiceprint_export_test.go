package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type exportBusyEngine struct{ engine }

func (exportBusyEngine) Busy() bool { return true }

func TestExportVoiceprintSamplesRejectsUnknownAndBusy(t *testing.T) {
	s, root := exportHarness(t)
	outside := filepath.Join(t.TempDir(), "secret.wav")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := s.ExportVoiceprintSamples(outside, dest); err == nil || !strings.Contains(err.Error(), "not in the current history") {
		t.Fatal(err)
	}
	if _, err := s.ExportVoiceprintSamples("missing-id", dest); err == nil {
		t.Fatal("unknown id was exported")
	}
	mic := writeExportClip(t, root, "daybee_20261009_140000_microphone.wav", "session-a", protocol.SourceMicrophone, []byte("mic-a"))
	s.engine = exportBusyEngine{engine: s.engine}
	if _, err := s.ExportVoiceprintSamples(mic, dest); err == nil || !strings.Contains(err.Error(), "Finish recording and transcription") {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 || s.store.Settings().Recording.RecordSystemAudio {
		t.Fatal("busy export created a sample or enabled system audio")
	}
}

func TestExportVoiceprintSamplesCopiesPairAndLeavesOriginals(t *testing.T) {
	s, root := exportHarness(t)
	micBody := []byte("microphone-bytes")
	systemBody := []byte("system-bytes")
	mic := writeExportClip(t, root, "daybee_20261009_140000_microphone.wav", "session-same", protocol.SourceMicrophone, micBody)
	system := writeExportClip(t, root, "daybee_20261009_140000_system.wav", "session-same", protocol.SourceSystem, systemBody)
	dest := t.TempDir()
	first, err := s.ExportVoiceprintSamples(mic, dest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ExportVoiceprintSamples(mic, dest)
	if err != nil || first == second {
		t.Fatalf("second export %s %v", second, err)
	}
	for _, path := range []string{mic, system} {
		body := micBody
		if path == system {
			body = systemBody
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("original changed %s %v", path, err)
		}
	}
	copied, err := os.ReadFile(filepath.Join(first, filepath.Base(system)))
	if err != nil || !bytes.Equal(copied, systemBody) {
		t.Fatal("system sibling with the same session was not copied", err)
	}
	raw, err := os.ReadFile(filepath.Join(first, "samples.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, hidden := range []string{"private transcript", "PREDICTED_ME", "embeddings", "profile"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("sidecar copied %s", hidden)
		}
	}
	var sidecar voiceprintSampleSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		t.Fatal(err)
	}
	if len(sidecar.IdentityRanges) != 0 || len(sidecar.Files) != 2 {
		t.Fatalf("sidecar %+v", sidecar)
	}
	for _, file := range sidecar.Files {
		if file.SessionID != "session-same" || file.Device != "native:export-mic" || file.EchoCancellation == nil || !*file.EchoCancellation || len(file.CapturedRanges) != 1 {
			t.Fatalf("capture metadata %+v", file)
		}
	}
	if s.store.Settings().Recording.RecordSystemAudio {
		t.Fatal("export enabled system audio saving")
	}
}

func TestExportVoiceprintSamplesSkipsOtherRotations(t *testing.T) {
	s, root := exportHarness(t)
	early := protocol.RecordingTimeRange{StartedAt: "2026-10-09T14:00:01Z", EndedAt: "2026-10-09T14:00:04Z"}
	late := protocol.RecordingTimeRange{StartedAt: "2026-10-09T18:00:01Z", EndedAt: "2026-10-09T18:00:04Z"}
	micEarly := writeExportClipRange(t, root, "daybee_20261009_140000_microphone.wav", "session-long", protocol.SourceMicrophone, []byte("mic-early"), early)
	systemEarly := writeExportClipRange(t, root, "daybee_20261009_140000_system.wav", "session-long", protocol.SourceSystem, []byte("system-early"), early)
	micLate := writeExportClipRange(t, root, "daybee_20261009_180000_microphone.wav", "session-long", protocol.SourceMicrophone, []byte("mic-late"), late)
	systemLate := writeExportClipRange(t, root, "daybee_20261009_180000_system.wav", "session-long", protocol.SourceSystem, []byte("system-late"), late)
	fromMic, err := s.ExportVoiceprintSamples(micEarly, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fromMic, filepath.Base(systemEarly))); err != nil {
		t.Fatal("same-segment system sibling was dropped", err)
	}
	if _, err := os.Stat(filepath.Join(fromMic, filepath.Base(systemLate))); !os.IsNotExist(err) {
		t.Fatal("later rotation in the same session was exported", err)
	}
	fromSystem, err := s.ExportVoiceprintSamples(systemLate, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fromSystem, filepath.Base(micLate))); err != nil {
		t.Fatal("matching microphone sibling was dropped", err)
	}
	if _, err := os.Stat(filepath.Join(fromSystem, filepath.Base(micEarly))); !os.IsNotExist(err) {
		t.Fatal("system export copied an earlier microphone rotation", err)
	}
}

func TestExportVoiceprintSamplesSkipsDifferentSession(t *testing.T) {
	s, root := exportHarness(t)
	mic := writeExportClip(t, root, "daybee_20261009_140000_microphone.flac", "session-mic", protocol.SourceMicrophone, []byte("mic"))
	system := writeExportClip(t, root, "daybee_20261009_140000_system.flac", "session-other", protocol.SourceSystem, []byte("other-system"))
	dir, err := s.ExportVoiceprintSamples(mic, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.Base(system))); !os.IsNotExist(err) {
		t.Fatal("same-second system file from another session was exported", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("export entries %d %v", len(entries), err)
	}
}

func TestExportVoiceprintSamplesRejectsSymlink(t *testing.T) {
	s, root := exportHarness(t)
	secret := filepath.Join(t.TempDir(), "secret.wav")
	if err := os.WriteFile(secret, []byte("secret-audio"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "daybee_20261009_140000_microphone.wav")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	writeExportMeta(t, link, "session-link", protocol.SourceMicrophone, protocol.RecordingTimeRange{StartedAt: "2026-10-09T14:00:01Z", EndedAt: "2026-10-09T14:00:04Z"})
	dest := t.TempDir()
	if _, err := s.ExportVoiceprintSamples(link, dest); err == nil || !strings.Contains(err.Error(), "regular WAV or FLAC") {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	got, _ := os.ReadFile(secret)
	if len(entries) != 0 || string(got) != "secret-audio" {
		t.Fatal("symlink export left a sample or changed the target")
	}
}

func TestExportVoiceprintSamplesCleansFailedCopy(t *testing.T) {
	s, root := exportHarness(t)
	body := []byte("keep-me")
	mic := writeExportClip(t, root, "daybee_20261009_150000_microphone.wav", "session-fail", protocol.SourceMicrophone, body)
	dest := t.TempDir()
	previous := copyVoiceprintFile
	t.Cleanup(func() { copyVoiceprintFile = previous })
	copyVoiceprintFile = func(string, string) error { return errors.New("copy failed") }
	if _, err := s.ExportVoiceprintSamples(mic, dest); err == nil || !strings.Contains(err.Error(), "copy failed") {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	got, err := os.ReadFile(mic)
	if len(entries) != 0 || err != nil || !bytes.Equal(got, body) {
		t.Fatalf("cleanup left %d entries, original %q %v", len(entries), got, err)
	}
}

func TestExportVoiceprintSamplesRejectsUnfinishedRecording(t *testing.T) {
	s, root := exportHarness(t)
	path := filepath.Join(root, "daybee_20261009_170000_microphone.wav")
	if err := os.WriteFile(path, []byte("RIFF"), 0600); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	if err := config.WriteJSON(base+".json", map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-09T17:00:00Z", "status": "recording",
		"recording_session_id": "session-open", "audio_source": protocol.SourceMicrophone,
	}); err != nil {
		t.Fatal(err)
	}
	if s.engine.Busy() {
		t.Fatal("engine should be idle")
	}
	dest := t.TempDir()
	if _, err := s.ExportVoiceprintSamples(path, dest); err == nil || !strings.Contains(err.Error(), "not finished") {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	got, _ := os.ReadFile(path)
	if len(entries) != 0 || string(got) != "RIFF" {
		t.Fatal("unfinished recording was exported or rewritten")
	}
}

func TestExportVoiceprintSamplesRejectsDirectorySymlinkEscape(t *testing.T) {
	s, root := exportHarness(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.wav")
	if err := os.WriteFile(secret, []byte("outside-audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(root, "linked", "secret.wav")
	if err := config.WriteJSON(filepath.Join(root, "forged.json"), map[string]any{
		"audio_path": escaped, "segment_start_time": "2026-10-09T14:00:00Z", "status": "ok",
		"recording_session_id": "session-escape", "audio_source": protocol.SourceMicrophone,
	}); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := s.ExportVoiceprintSamples(escaped, dest); err == nil {
		t.Fatal("exported a file outside the recording root")
	}
	entries, _ := os.ReadDir(dest)
	got, _ := os.ReadFile(secret)
	if len(entries) != 0 || string(got) != "outside-audio" {
		t.Fatal("directory symlink export left a sample or changed the external file")
	}
}

func TestExportVoiceprintSamplesAllowsRootAlias(t *testing.T) {
	s, root := exportHarness(t)
	alias := filepath.Join(t.TempDir(), "recordings-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	body := []byte("alias-audio")
	real := writeExportClip(t, root, "daybee_20261009_160000_microphone.wav", "session-alias", protocol.SourceMicrophone, body)
	aliased := filepath.Join(alias, filepath.Base(real))
	if !voiceprintInsideRoots(real, []string{alias}) || !voiceprintInsideRoots(aliased, []string{alias}) {
		t.Fatal("a root alias should resolve to the same recordings directory")
	}
	writeExportMeta(t, aliased, "session-alias", protocol.SourceMicrophone, protocol.RecordingTimeRange{StartedAt: "2026-10-09T14:00:01Z", EndedAt: "2026-10-09T14:00:04Z"})
	dest := t.TempDir()
	dir, err := s.ExportVoiceprintSamples(aliased, dest)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(dir, filepath.Base(real)))
	if err != nil || !bytes.Equal(copied, body) {
		t.Fatalf("alias export copied %q: %v", copied, err)
	}
}

func exportHarness(t *testing.T) (*DesktopService, string) {
	t.Helper()
	root := t.TempDir()
	store, err := config.Open(t.TempDir(), t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.Recording.OutputDir = root
	settings.Recording.RecordSystemAudio = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	close(ready)
	engine := newTestEngine(store.Settings(), func() {})
	return &DesktopService{ready: ready, store: store, engine: engine, test: true}, root
}

func writeExportClip(t *testing.T, dir, name, session string, source protocol.AudioSource, body []byte) string {
	t.Helper()
	return writeExportClipRange(t, dir, name, session, source, body, protocol.RecordingTimeRange{StartedAt: "2026-10-09T14:00:01Z", EndedAt: "2026-10-09T14:00:04Z"})
}

func writeExportClipRange(t *testing.T, dir, name, session string, source protocol.AudioSource, body []byte, span protocol.RecordingTimeRange) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	writeExportMeta(t, path, session, source, span)
	return path
}

func writeExportMeta(t *testing.T, path, session string, source protocol.AudioSource, span protocol.RecordingTimeRange) {
	t.Helper()
	base := strings.TrimSuffix(path, filepath.Ext(path))
	if err := config.WriteJSON(base+".json", map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-09T14:00:00Z", "status": "ok",
		"recording_session_id": session, "audio_source": source, "text": "private transcript",
		"transcript_chunks": []protocol.TranscriptChunk{{Text: "private transcript", VoiceIdentity: "PREDICTED_ME", VoiceScore: 0.99}},
		"embeddings":        []float32{1, 2, 3},
		"profile":           "saved-profile",
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteJSON(base+".recording.json", map[string]any{
		"audio_path": path, "segment_start_time": "2026-10-09T14:00:00.5Z", "recording_session_id": session,
		"audio_source": source, "capture_device_id": "native:export-mic", "echo_cancellation": true,
		"captured_ranges": []protocol.RecordingTimeRange{span},
	}); err != nil {
		t.Fatal(err)
	}
}
