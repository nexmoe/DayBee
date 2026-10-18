package recorder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
)

func TestHistoryReusesLegacyMetadataAndDeduplicatesAudio(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "daybee_20261009_120000")
	for _, ext := range []string{".wav", ".flac"} {
		if err := os.WriteFile(base+ext, []byte("audio"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.WriteJSON(base+".json", map[string]any{"audio_path": base + ".flac", "segment_start_time": "2026-10-09T12:00:00+08:00", "input_device": "旧设备", "status": "ok", "text": "  原有转写  "}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "moved", "daybee_20261009_130000.wav")
	if err := config.WriteJSON(filepath.Join(root, "retained.json"), map[string]any{"audio_path": missing, "created_at": "2026-10-09T13:00:00+08:00", "text": "保留历史"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root, root})
	if len(items) != 2 || items[0].AudioPath != missing || items[1].AudioPath != base+".flac" || items[1].TextPreview != "原有转写" {
		t.Fatal("legacy history changed", items)
	}
	if items[1].InputDevice == nil || *items[1].InputDevice != "旧设备" {
		t.Fatal("legacy device missing")
	}
}

func TestLegacyBatchTimeDoesNotReplaceRecordingFilenameTime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "daybee_20261009_111248.flac")
	if err := config.WriteJSON(filepath.Join(root, "daybee_20261009_111248.json"), map[string]any{
		"audio_path": path, "created_at": "2026-10-09T12:56:20+08:00", "text": "原录音的转写",
	}); err != nil {
		t.Fatal(err)
	}
	item := History([]string{root})[0]
	if item.StartedAt != "2026-10-09T11:12:48" {
		t.Fatalf("batch completion moved the original recording time: %+v", item)
	}
}

func TestArchiveScansIgnoreStagedFLAC(t *testing.T) {
	root := t.TempDir()
	complete := filepath.Join(root, "daybee_20261018_120000.flac")
	for _, path := range []string{complete, filepath.Join(root, ".daybee-123.flac")} {
		if err := os.WriteFile(path, []byte("audio"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := AudioFiles([]string{root})
	if err != nil || len(files) != 1 || files[0] != complete {
		t.Errorf("batch scan included an unpublished FLAC: %v, %v", files, err)
	}
	items := History([]string{root})
	if len(items) != 1 || items[0].AudioPath != complete {
		t.Errorf("history included an unpublished FLAC: %+v", items)
	}
}
