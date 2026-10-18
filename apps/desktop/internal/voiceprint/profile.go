package voiceprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
)

const ModelName = "3dspeaker_speech_eres2netv2_sv_zh-cn_16k-common.onnx"
const embeddingDimension = 192
const matchThreshold float32 = .6

type Profile struct {
	Version            int                 `json:"version"`
	Model              string              `json:"model"`
	Embeddings         [][]float32         `json:"embeddings"`
	CreatedAt          string              `json:"createdAt"`
	Device             string              `json:"device,omitempty"`
	EchoCancellation   bool                `json:"echoCancellation"`
	Calibration        *Calibration        `json:"calibration,omitempty"`
	Adaptation         [][]float32         `json:"adaptation,omitempty"`
	AdaptationProgress *AdaptationProgress `json:"adaptationProgress,omitempty"`
	Revision           uint64              `json:"-"`
}

func clone(p *Profile) *Profile {
	if p == nil {
		return nil
	}
	copy := *p
	copy.Embeddings = make([][]float32, len(p.Embeddings))
	for i, vector := range p.Embeddings {
		copy.Embeddings[i] = append([]float32(nil), vector...)
	}
	if p.Calibration != nil {
		calibration := *p.Calibration
		copy.Calibration = &calibration
	}
	copy.Adaptation = copySystem(p.Adaptation)
	if p.AdaptationProgress != nil {
		progress := *p.AdaptationProgress
		copy.AdaptationProgress = &progress
	}
	return &copy
}

func validEmbedding(vector []float32) bool {
	if len(vector) != embeddingDimension {
		return false
	}
	var norm float64
	for _, value := range vector {
		if !finite32(value) {
			return false
		}
		norm += float64(value) * float64(value)
	}
	return norm > 0
}

func validVectors(vectors [][]float32) bool {
	if len(vectors) == 0 || len(vectors) > 64 {
		return false
	}
	for _, vector := range vectors {
		if !validEmbedding(vector) {
			return false
		}
	}
	return true
}

func profileAccepted(profile *Profile) bool {
	if profile == nil || profile.Model != ModelName || !validVectors(profile.Embeddings) || !adaptationOK(profile.Adaptation) {
		return false
	}
	switch profile.Version {
	case 1, 2:
		return true
	case calibratedVersion:
		return calibrationOK(profile)
	default:
		return false
	}
}

func calibrationOK(profile *Profile) bool {
	if profile == nil || profile.Calibration == nil || profile.Calibration.CohortID == "" || !bandOK(profile.Calibration) {
		return false
	}
	return resolvedDevice(profile.Device)
}

// A corrupt file is retained for recovery; ordinary transcription can proceed.
func New(root string, modelPath func() string) (*Manager, error) {
	m := &Manager{path: filepath.Join(root, "voiceprint", "profile.json"), modelPath: modelPath}
	raw, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return m, err
	}
	if !profileAccepted(&p) {
		return m, errors.New("The saved voiceprint is invalid. Please initialize it again.")
	}
	m.profile = clone(&p)
	m.revision = 1
	m.profile.Revision = 1
	return m, nil
}

func (m *Manager) Snapshot() *Profile {
	if m == nil {
		return nil
	}
	m.profileMu.RLock()
	defer m.profileMu.RUnlock()
	return clone(m.profile)
}

func (m *Manager) Enrolled() bool { return m.Snapshot() != nil }

func (m *Manager) Save(embeddings [][]float32) error {
	if m == nil || !validVectors(embeddings) {
		return errors.New("A valid speaker embedding is required.")
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.closed.Load() {
		return errors.New("Voiceprint manager is closed.")
	}
	p := clone(&Profile{Version: 2, Model: ModelName, Embeddings: embeddings, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	if err := config.WriteJSON(m.path, p); err != nil {
		return err
	}
	m.publishLocked(p)
	return nil
}

func (m *Manager) SaveProfile(profile *Profile) error {
	if m == nil || !profileAccepted(profile) {
		return errors.New("A valid speaker embedding is required.")
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if m.closed.Load() {
		return errors.New("Voiceprint manager is closed.")
	}
	stored := clone(profile)
	if stored.CreatedAt == "" {
		stored.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if stored.Version >= calibratedVersion {
		stored.Device = strings.TrimSpace(stored.Device)
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	if err := config.WriteJSON(m.path, stored); err != nil {
		return err
	}
	m.publishLocked(stored)
	return nil
}

func (m *Manager) Clear() error {
	if m == nil {
		return nil
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m.publishLocked(nil)
	return nil
}
