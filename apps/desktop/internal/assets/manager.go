package assets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/speechanalyzer"
)

const releaseRoot = "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/"

type Progress struct {
	Downloading bool
	Message     string
	Percent     *float64
	VADReady    bool
	ASRReady    bool
}

type Manager struct {
	vadGate       chan struct{}
	asrGate       chan struct{}
	voiceGate     chan struct{}
	active        atomic.Int64
	selected      atomic.Value
	downloadModel atomic.Value
	root          string
	roots         []string
	client        *http.Client
	preferChina   bool
	idleTimeout   time.Duration
	progress      func(Progress)
}

func New(root string, legacy []string, progress func(Progress)) *Manager {
	roots := []string{filepath.Join(root, "models")}
	for _, old := range legacy {
		roots = append(roots, filepath.Join(old, "models"))
	}
	return &Manager{root: roots[0], roots: roots,
		client: &http.Client{Timeout: time.Hour}, progress: progress,
		preferChina: preferChinaMirrors(), idleTimeout: 15 * time.Second,
		vadGate: make(chan struct{}, 1), asrGate: make(chan struct{}, 1), voiceGate: make(chan struct{}, 1)}
}

func usableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func validModel(path string) bool {
	model, ok := asrmodel.ForDirectory(path)
	return ok && model.Ready(path)
}

type selection struct {
	id       protocol.ASRModel
	language string
}

func (m *Manager) SelectASRModel(id protocol.ASRModel, language ...string) {
	if !protocol.ValidASRModel(id) {
		id = protocol.DefaultASRModel
	}
	m.selected.Store(selection{id, m.modelLanguage(language)})
}

func (m *Manager) selectedModel() protocol.ASRModel {
	if id := m.selected.Load(); id != nil {
		return id.(selection).id
	}
	return protocol.DefaultASRModel
}

func (m *Manager) modelLanguage(language []string) string {
	if len(language) > 0 && language[0] != "" {
		return language[0]
	}
	if selected := m.selected.Load(); selected != nil {
		return selected.(selection).language
	}
	return "auto"
}

func (m *Manager) VADPath() string {
	for _, root := range m.roots {
		path := filepath.Join(root, "silero_vad.onnx")
		if usableFile(path) {
			return path
		}
	}
	return filepath.Join(m.root, "silero_vad.onnx")
}

func (m *Manager) ASRPath() string {
	return m.ASRModelPath(m.selectedModel())
}

func (m *Manager) ASRModelPath(id protocol.ASRModel, language ...string) string {
	if id == protocol.ASRSpeechAnalyzer {
		return speechanalyzer.ModelKey(m.modelLanguage(language))
	}
	model, _ := asrmodel.Find(id)
	return m.modelPath(model)
}

func (m *Manager) modelPath(model asrmodel.Model) string {
	for _, root := range m.roots {
		path := filepath.Join(root, model.Directory)
		if model.Ready(path) {
			return path
		}
	}
	return filepath.Join(m.root, model.Directory)
}

func (m *Manager) Ready() (vad, asr bool) {
	if m.selectedModel() == protocol.ASRSpeechAnalyzer {
		return usableFile(m.VADPath()), speechanalyzer.Ready(m.modelLanguage(nil))
	}
	return usableFile(m.VADPath()), validModel(m.ASRPath())
}

func (m *Manager) publish(downloading bool, message string, percent *float64) {
	if m.progress == nil {
		return
	}
	vad, asr := m.Ready()
	m.progress(Progress{downloading || m.active.Load() > 0, message, percent, vad, asr})
}

// Each model coalesces its own downloads. Preparing VAD must never wait for a
// background ASR download, and cancellation also interrupts waiting for a gate.
func (m *Manager) Ensure(ctx context.Context, needVAD, needASR bool) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	if needVAD {
		if err := m.withGate(ctx, m.vadGate, func() error {
			if usableFile(m.VADPath()) {
				return nil
			}
			return m.download(ctx, releaseRoot+"silero_vad.onnx", filepath.Join(m.root, "silero_vad.onnx"), "Downloading VAD model")
		}); err != nil {
			return err
		}
	}
	if needASR {
		return m.EnsureASRModel(ctx, m.selectedModel())
	}
	return nil
}

func (m *Manager) withGate(ctx context.Context, gate chan struct{}, fn func() error) error {
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	m.active.Add(1)
	defer func() { m.active.Add(-1); <-gate; m.publish(false, "", nil) }()
	return fn()
}

func (m *Manager) EnsureASRModel(ctx context.Context, id protocol.ASRModel, language ...string) error {
	model, ok := asrmodel.Find(id)
	if !ok || !protocol.ValidASRModel(id) {
		return errors.New("Unknown ASR model")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	return m.withGate(ctx, m.asrGate, func() error {
		m.downloadModel.Store(id)
		defer m.downloadModel.Store(protocol.ASRModel(""))
		if id == protocol.ASRSpeechAnalyzer {
			m.publish(true, "Preparing SpeechAnalyzer system model…", nil)
			return speechanalyzer.Ensure(ctx, m.modelLanguage(language))
		}
		return m.ensureASR(ctx, model)
	})
}

func (m *Manager) Models() protocol.ASRModelsSnapshot {
	result := protocol.ASRModelsSnapshot{Items: []protocol.ASRModelSnapshot{}}
	for _, model := range asrmodel.All() {
		ready := false
		if model.ID == protocol.ASRSpeechAnalyzer {
			ready = speechanalyzer.Ready(m.modelLanguage(nil))
		} else {
			ready = model.Ready(m.modelPath(model))
		}
		result.Items = append(result.Items, protocol.ASRModelSnapshot{ID: model.ID, Name: model.Name, EstimatedBytes: model.EstimatedBytes, Ready: ready})
	}
	if value := m.downloadModel.Load(); value != nil && value.(protocol.ASRModel) != "" {
		id := value.(protocol.ASRModel)
		result.DownloadingModel = &id
	}
	return result
}

func (m *Manager) ensureASR(ctx context.Context, spec asrmodel.Model) error {
	if !spec.Ready(m.modelPath(spec)) {
		archive, err := os.CreateTemp(m.root, ".asr-*.tar.bz2")
		if err != nil {
			return err
		}
		path := archive.Name()
		archive.Close()
		os.Remove(path)
		defer os.Remove(path)
		if err := m.download(ctx, releaseRoot+spec.Directory+".tar.bz2", path, "Downloading "+spec.Name); err != nil {
			return err
		}
		m.publish(true, "Extracting "+spec.Name+"…", nil)
		stage, err := os.MkdirTemp(m.root, ".asr-extract-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := extractModel(ctx, path, stage); err != nil {
			return err
		}
		model := filepath.Join(stage, spec.Directory)
		if !spec.Ready(model) {
			return fmt.Errorf("Downloaded %s model is incomplete.", spec.Name)
		}
		if err := pruneWhisperFP32(spec, model); err != nil {
			return err
		}
		target := filepath.Join(m.root, spec.Directory)
		// Keep an incomplete previous directory until the replacement is ready.
		old := filepath.Join(stage, "previous-incomplete-model")
		if _, err := os.Stat(target); err == nil {
			if err := os.Rename(target, old); err != nil {
				return err
			}
		}
		if err := os.Rename(model, target); err != nil {
			_ = os.Rename(old, target)
			return err
		}
		_ = os.RemoveAll(old)
	}
	return nil
}
