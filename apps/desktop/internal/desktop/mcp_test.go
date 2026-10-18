package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/config"
)

func TestMCPHistoryFollowsSavedOutputAndReadsLiveTranscriptsWithoutWritingSettings(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "desktop-settings.json")
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	defaults, legacy := filepath.Join(root, "default"), filepath.Join(root, "legacy")
	write := func(directory, text string) {
		t.Helper()
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := config.WriteJSON(filepath.Join(directory, "segment.json"), map[string]any{
			"audio_path": filepath.Join(directory, "segment.wav"), "segment_start_time": "2026-10-09T12:00:00+08:00", "text": text,
		}); err != nil {
			t.Fatal(err)
		}
	}
	save := func(output string) {
		t.Helper()
		if err := config.WriteJSON(settingsPath, map[string]any{"settings": map[string]any{"recording": map[string]any{"outputDir": output}}}); err != nil {
			t.Fatal(err)
		}
	}
	write(first, "Current folder")
	write(defaults, "Default folder")
	write(legacy, "Legacy folder")
	save(first)
	before, _ := os.ReadFile(settingsPath)
	source := mcpHistorySource{settingsPath: settingsPath, defaultOutput: defaults, legacyOutputs: []string{legacy}}
	items, err := source.read(context.Background())
	if err != nil || len(items) != 3 {
		t.Fatalf("history=%+v err=%v", items, err)
	}
	after, _ := os.ReadFile(settingsPath)
	if string(before) != string(after) {
		t.Fatal("MCP changed the desktop configuration")
	}
	write(second, "New output")
	save(second)
	write(second, "New output plus newly completed speech")
	items, err = source.read(context.Background())
	if err != nil || len(items) != 3 {
		t.Fatalf("updated history=%+v err=%v", items, err)
	}
	found := false
	for _, item := range items {
		if item.TextPreview == "Current folder" {
			t.Fatal("old custom output directory remained exposed")
		}
		found = found || strings.Contains(item.TextPreview, "newly completed speech")
	}
	if !found {
		t.Fatal("newly completed speech was not read")
	}
}

func TestMCPHistoryDoesNotCreateOrRepairConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing", "desktop-settings.json")
	source := mcpHistorySource{settingsPath: path, defaultOutput: filepath.Join(root, "recordings")}
	if items, err := source.read(context.Background()); err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("MCP created configuration directories")
	}
	path = filepath.Join(root, "desktop-settings.json")
	source.settingsPath = path
	for _, invalid := range []string{"broken", "null", `{"settings":null}`, `{"settings":[]}`} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := source.read(context.Background()); err == nil {
			t.Fatal("corrupt settings accepted", invalid)
		}
		data, _ := os.ReadFile(path)
		if string(data) != invalid {
			t.Fatal("MCP rewrote invalid configuration")
		}
	}
}

func TestMCPConfigurationPreservesExecutablePathsAndIsolation(t *testing.T) {
	for _, store := range []string{"", filepath.Join(t.TempDir(), "隔离数据")} {
		executable := filepath.Join(t.TempDir(), "DayBee App", "DayBee")
		text, err := mcpConfiguration(executable, store)
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Servers map[string]struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal([]byte(text), &config); err != nil {
			t.Fatal(err)
		}
		server := config.Servers["daybee"]
		if server.Command != executable || len(server.Args) != 1 || server.Args[0] != "--mcp" || server.Env["DAYBEE_STORE_DIR"] != store {
			t.Fatal("configuration changed paths or scope", text)
		}
		if store == "" && server.Env != nil {
			t.Fatal("normal clients should use the shared production store")
		}
	}
}
