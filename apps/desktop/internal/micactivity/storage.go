package micactivity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
)

func (m *Monitor) load() error {
	files, err := filepath.Glob(filepath.Join(m.directory, "????????.json"))
	if err != nil {
		return err
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var entries []entry
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("Read microphone history %s: %w", filepath.Base(path), err)
		}
		day := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, item := range entries {
			if _, err := time.Parse(time.RFC3339Nano, item.StartedAt); err != nil || item.ID == "" || item.AppID == "" {
				return fmt.Errorf("Invalid microphone history in %s", filepath.Base(path))
			}
			if !identifiedApplication(item.AppID) {
				m.dirtyDays[day] = true
				continue
			}
			item.Day = day
			if item.EndedAt == nil {
				// A previous process did not shut down cleanly. Only claim the
				// interval it actually observed, never the time DayBee was absent.
				ended := item.LastObservedAt
				if _, err := time.Parse(time.RFC3339Nano, ended); err != nil || timeBefore(ended, item.StartedAt) {
					ended = item.StartedAt
				}
				item.EndedAt = &ended
				m.dirtyDays[day] = true
			}
			m.entries = append(m.entries, item)
		}
	}
	return nil
}

func (m *Monitor) save() error {
	if len(m.dirtyDays) > 0 {
		if err := os.MkdirAll(m.directory, 0700); err != nil {
			return fmt.Errorf("Save microphone history: %w", err)
		}
	}
	for day := range m.dirtyDays {
		entries := []entry{}
		for _, item := range m.entries {
			if item.Day == day {
				entries = append(entries, item)
			}
		}
		var err error
		if len(entries) == 0 {
			err = os.Remove(m.dayPath(day))
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = config.WriteJSON(m.dayPath(day), entries)
		}
		if err != nil {
			return fmt.Errorf("Save microphone history: %w", err)
		}
		delete(m.dirtyDays, day)
	}
	return nil
}
