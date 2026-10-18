// Package micactivity observes other applications' microphone use without
// opening an audio stream or recording their audio.
package micactivity

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

var ErrUnsupported = errors.New("Microphone application detection is unavailable on this system")

type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	PID  int    `json:"pid"`
}

type Scan func() ([]Application, error)

type entry struct {
	protocol.MicrophoneUsage
	LastObservedAt  string `json:"lastObservedAt"`
	RecordingLinked bool   `json:"recordingLinked,omitempty"`
	Day             string `json:"-"`
}

type Monitor struct {
	mu           sync.Mutex
	directory    string
	scan         Scan
	selfPID      int
	entries      []entry
	active       map[string]int
	dirtyDays    map[string]bool
	checkpoint   time.Time
	supported    bool
	scanError    error
	storageError error
	suspended    bool
	closed       bool
}

func Open(directory string, scan Scan, selfPID int) (*Monitor, error) {
	m := &Monitor{directory: directory, scan: scan, selfPID: selfPID,
		active: map[string]int{}, dirtyDays: map[string]bool{}, supported: true}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, m.save()
}

// Poll is serialized with suspend and shutdown. Failed observations never end
// sessions: an unavailable audio service is not evidence that an app stopped.
func (m *Monitor) Poll(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.suspended {
		return
	}
	apps, err := m.scan()
	if err != nil {
		m.supported = !errors.Is(err, ErrUnsupported)
		m.scanError = err
		return
	}
	m.supported, m.scanError = true, nil
	seen := map[string]bool{}
	at := now.UTC().Format(time.RFC3339Nano)
	changed := false
	for _, app := range apps {
		if app.PID == m.selfPID || app.PID <= 0 || !identifiedApplication(app.ID) || seen[app.ID] {
			continue
		}
		seen[app.ID] = true
		index, exists := m.active[app.ID]
		if !exists {
			index = len(m.entries)
			m.entries = append(m.entries, entry{MicrophoneUsage: protocol.MicrophoneUsage{
				ID: fmt.Sprintf("%x", randomID()), AppID: app.ID, AppName: app.Name,
				AppIcon: app.Icon, StartedAt: at}, Day: now.Format("20060102")})
			m.active[app.ID] = index
			changed = true
		}
		item := &m.entries[index]
		if item.LastObservedAt == "" || !timeBefore(at, item.LastObservedAt) {
			item.LastObservedAt = at
		}
		m.dirtyDays[item.Day] = true
	}
	for appID, index := range m.active {
		if !seen[appID] {
			m.end(index, at)
			delete(m.active, appID)
			changed = true
		}
	}
	// Persist transitions immediately, and checkpoint ongoing sessions so a
	// crash can be recovered without extending usage through app downtime.
	if changed || now.Sub(m.checkpoint) >= 5*time.Second {
		m.storageError = m.save()
		m.checkpoint = now
	}
}

func randomID() []byte {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return id[:]
}

func (m *Monitor) end(index int, at string) {
	item := &m.entries[index]
	if timeBefore(at, item.LastObservedAt) {
		at = item.LastObservedAt
	}
	if timeBefore(at, item.StartedAt) {
		at = item.StartedAt
	}
	item.EndedAt = &at
	m.dirtyDays[item.Day] = true
}

func (m *Monitor) Suspend(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.suspended = true
	return m.finish(now)
}

func (m *Monitor) Resume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.suspended = false
}

func (m *Monitor) Close(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.finish(now)
}

func (m *Monitor) finish(now time.Time) error {
	for _, index := range m.active {
		m.end(index, now.UTC().Format(time.RFC3339Nano))
	}
	clear(m.active)
	m.storageError = m.save()
	return m.storageError
}

func (m *Monitor) Snapshot() protocol.MicrophoneActivitySnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := protocol.MicrophoneActivitySnapshot{Supported: m.supported, Items: []protocol.MicrophoneUsage{}}
	if err := errors.Join(m.storageError, m.scanError); err != nil && m.supported {
		message := err.Error()
		snapshot.Error = &message
	}
	for _, item := range m.entries {
		usage := item.MicrophoneUsage
		if usage.EndedAt != nil {
			ended := *usage.EndedAt
			usage.EndedAt = &ended
		}
		snapshot.Items = append(snapshot.Items, usage)
	}
	sort.SliceStable(snapshot.Items, func(i, j int) bool { return timeBefore(snapshot.Items[j].StartedAt, snapshot.Items[i].StartedAt) })
	return snapshot
}

func (m *Monitor) dayPath(day string) string { return filepath.Join(m.directory, day+".json") }

func timeBefore(a, b string) bool {
	left, _ := time.Parse(time.RFC3339Nano, a)
	right, _ := time.Parse(time.RFC3339Nano, b)
	return left.Before(right)
}
