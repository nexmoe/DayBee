package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/recorder"
	"github.com/nexmoe/eve/apps/desktop/internal/transcriptmcp"
)

// RunMCP runs without starting the desktop service, recording engine or windows.
func RunMCP(ctx context.Context) error {
	appData, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		return err
	}
	documents, err := mygo.App.Path(mygo.PathDocuments)
	if err != nil {
		return err
	}
	service := &DesktopService{test: os.Getenv("DAYBEE_E2E_TEST") == "1"}
	paths := service.dataPaths(appData, documents)
	source := mcpHistorySource{settingsPath: filepath.Join(paths.root, "desktop-settings.json"),
		defaultOutput: filepath.Join(paths.documents, AppName)}
	if !service.isolated() {
		source.legacyOutputs = []string{filepath.Join(documents, "RecBee"), filepath.Join(documents, "Eve Recorder")}
	}
	return transcriptmcp.NewServer(mygo.App.Version(), source.read).Run(ctx, &mcp.StdioTransport{})
}

type mcpHistorySource struct {
	settingsPath  string
	defaultOutput string
	legacyOutputs []string
}

func (s mcpHistorySource) read(ctx context.Context) ([]protocol.RecordingHistoryItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output := s.defaultOutput
	data, err := os.ReadFile(s.settingsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read DayBee settings: %w", err)
	}
	if err == nil {
		var document struct {
			Settings *struct {
				Recording struct {
					OutputDir string `json:"outputDir"`
				} `json:"recording"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(data, &document); err != nil || document.Settings == nil {
			return nil, errors.New("DayBee settings are invalid; open DayBee to repair the configuration")
		}
		if filepath.IsAbs(document.Settings.Recording.OutputDir) {
			output = document.Settings.Recording.OutputDir
		}
	}
	// Read the current output directory on every request. The app writes these
	// files atomically, including completed speech while recording continues.
	items := recorder.History(append([]string{output, s.defaultOutput}, s.legacyOutputs...))
	return items, ctx.Err()
}

func (s *DesktopService) GetMCPConfig() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return mcpConfiguration(executable, os.Getenv("DAYBEE_STORE_DIR"))
}

func (s *DesktopService) CopyMCPConfig() (string, error) {
	configuration, err := s.GetMCPConfig()
	if err != nil {
		return "", err
	}
	mygo.Clipboard.WriteText(configuration)
	return configuration, nil
}

func mcpConfiguration(executable, storeDir string) (string, error) {
	connection := map[string]any{"command": executable, "args": []string{"--mcp"}}
	if storeDir != "" {
		absolute, err := filepath.Abs(storeDir)
		if err != nil {
			return "", err
		}
		connection["env"] = map[string]string{"DAYBEE_STORE_DIR": absolute}
	}
	data, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"daybee": connection}}, "", "  ")
	return string(data), err
}
