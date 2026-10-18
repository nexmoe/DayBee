package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type WindowState struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

type document struct {
	Settings      protocol.AppSettings `json:"settings"`
	WindowState   WindowState          `json:"windowState"`
	NativeDevices bool                 `json:"nativeDevices,omitempty"`
}

type Store struct {
	mu       sync.Mutex
	root     string
	defaults protocol.AppSettings
	data     document
}

// Open prefers a valid canonical configuration, then imports a valid legacy
// configuration. Models and audio stay in place for rollback to Electron.
func Open(root, documents string, legacy []string, migrateDevices bool) (*Store, error) {
	s := &Store{root: root, defaults: protocol.DefaultSettings(filepath.Join(documents, "DayBee"))}
	s.data = document{Settings: s.defaults, WindowState: WindowState{620, 420}, NativeDevices: true}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	paths := append([]string{root}, legacy...)
	var invalid error
	for _, directory := range paths {
		path := filepath.Join(directory, "desktop-settings.json")
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var payload map[string]json.RawMessage
		var settingsObject map[string]json.RawMessage
		decodeErr := json.Unmarshal(raw, &payload)
		settingsErr := json.Unmarshal(payload["settings"], &settingsObject)
		if decodeErr != nil || settingsErr != nil || settingsObject == nil {
			invalid = fmt.Errorf("read settings %s: invalid configuration", path)
			if directory == root {
				if err := writeExclusive(filepath.Join(root, "desktop-settings.corrupt-backup.json"), raw); err != nil {
					return nil, err
				}
			}
			slog.Warn("Skipping invalid settings", "path", path)
			continue
		}
		s.data.Settings = normalize(payload["settings"], s.defaults)
		_ = json.Unmarshal(payload["windowState"], &s.data.WindowState)
		if s.data.WindowState.Width < 380 {
			s.data.WindowState.Width = 420
		}
		if s.data.WindowState.Width > 520 {
			s.data.WindowState.Width = 520
		}
		if s.data.WindowState.Height < 480 {
			s.data.WindowState.Height = 620
		}
		var alreadyNative bool
		_ = json.Unmarshal(payload["nativeDevices"], &alreadyNative)
		changed := directory != root || !alreadyNative
		if migrateDevices && !alreadyNative && s.data.Settings.Recording.Device != "default" {
			slog.Info("resetting legacy browser microphone ID to default")
			s.data.Settings.Recording.Device = "default"
			changed = true
		}
		if changed {
			backup := filepath.Join(root, "desktop-settings.electron-backup.json")
			if err := writeExclusive(backup, raw); err != nil {
				return nil, err
			}
			if err := s.saveLocked(); err != nil {
				return nil, err
			}
		}
		return s, nil
	}
	if invalid != nil {
		return nil, invalid
	}
	return s, s.saveLocked()
}

func (s *Store) Settings() protocol.AppSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

func (s *Store) Save(settings protocol.AppSettings) (protocol.AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(settings)
	if err != nil {
		return s.data.Settings, err
	}
	next := normalize(raw, s.defaults)
	previous := s.data.Settings
	s.data.Settings = next
	if err := s.saveLocked(); err != nil {
		s.data.Settings = previous
		return previous, err
	}
	return next, nil
}

func (s *Store) Window() WindowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.WindowState
}

func (s *Store) SaveWindow(state WindowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.data.WindowState
	s.data.WindowState = state
	if err := s.saveLocked(); err != nil {
		s.data.WindowState = previous
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	return WriteJSON(filepath.Join(s.root, "desktop-settings.json"), s.data)
}

func normalize(raw []byte, defaults protocol.AppSettings) protocol.AppSettings {
	base, _ := json.Marshal(defaults)
	var dst, src map[string]any
	_ = json.Unmarshal(base, &dst)
	_ = json.Unmarshal(raw, &src)
	merge(dst, src)
	merged, _ := json.Marshal(dst)
	result := defaults
	_ = json.Unmarshal(merged, &result)
	result.Desktop.HideWindowOnClose = true
	if !protocol.SupportsEchoCancellation {
		result.Recording.EchoCancellation = false
	}
	if !protocol.SupportsSystemAudio {
		result.Recording.RecordSystemAudio = false
	}
	switch result.Desktop.Language {
	case protocol.LanguageEnglish, protocol.LanguageChinese, protocol.LanguageSystem:
	default:
		result.Desktop.Language = defaults.Desktop.Language
	}
	switch result.Desktop.Theme {
	case protocol.ThemeDark, protocol.ThemeLight, protocol.ThemeSystem:
	default:
		result.Desktop.Theme = defaults.Desktop.Theme
	}
	switch result.Desktop.VoiceprintSetup {
	case protocol.VoiceprintPending, protocol.VoiceprintSkipped, protocol.VoiceprintConfigured:
	default:
		result.Desktop.VoiceprintSetup = protocol.VoiceprintPending
	}
	if result.Recording.AudioFormat != protocol.WAV {
		result.Recording.AudioFormat = protocol.FLAC
	}
	if !protocol.ValidASRModel(result.Recording.ASRModel) {
		result.Recording.ASRModel = protocol.DefaultASRModel
	}
	if !filepath.IsAbs(result.Recording.OutputDir) {
		result.Recording.OutputDir = defaults.Recording.OutputDir
	}
	if result.Recording.AutoSwitchConfirmations < 1 {
		result.Recording.AutoSwitchConfirmations = 2
	}
	if result.Recording.SegmentMinutes <= 0 {
		result.Recording.SegmentMinutes = 60
	}
	return result
}

func merge(dst, src map[string]any) {
	for key, value := range src {
		switch fallback := dst[key].(type) {
		case map[string]any:
			if child, ok := value.(map[string]any); ok {
				merge(fallback, child)
			}
		case string:
			if v, ok := value.(string); ok && strings.TrimSpace(v) != "" {
				dst[key] = v
			}
		case bool:
			if v, ok := value.(bool); ok {
				dst[key] = v
			}
		case float64:
			if v, ok := value.(float64); ok {
				dst[key] = v
			}
		}
	}
}

func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}
