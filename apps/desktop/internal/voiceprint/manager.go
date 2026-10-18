// Package voiceprint annotates speaker identity without filtering audio or text.
package voiceprint

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/eve/apps/desktop/internal/infer"
)

type Manager struct {
	mu        sync.Mutex
	profileMu sync.RWMutex
	path      string
	modelPath func() string
	profile   *Profile
	revision  uint64
	runtime   runtimeState
	extractor *sherpa.SpeakerEmbeddingExtractor
	closed    atomic.Bool
}

func (m *Manager) embedLocked(samples []float32, rate int) ([]float32, error) {
	if m.closed.Load() || rate != voiceSampleRate || len(samples) < minWindowSamples {
		return nil, errors.New("At least 0.8 seconds of 16 kHz speech is required.")
	}
	if m.extractor == nil {
		if m.modelPath == nil {
			return nil, errors.New("Voiceprint model is unavailable.")
		}
		path := m.modelPath()
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, errors.New("Voiceprint model is unavailable.")
		}
		m.extractor = sherpa.NewSpeakerEmbeddingExtractor(&sherpa.SpeakerEmbeddingExtractorConfig{Model: path, NumThreads: infer.Threads(), Provider: "cpu"})
		if m.extractor == nil || m.extractor.Dim() != embeddingDimension {
			if m.extractor != nil {
				sherpa.DeleteSpeakerEmbeddingExtractor(m.extractor)
				m.extractor = nil
			}
			return nil, errors.New("Failed to initialize the voiceprint model.")
		}
	}
	stream := m.extractor.CreateStream()
	if stream == nil {
		return nil, errors.New("Failed to create a voiceprint stream.")
	}
	defer sherpa.DeleteOnlineStream(stream)
	stream.AcceptWaveform(rate, samples)
	stream.InputFinished()
	if !m.extractor.IsReady(stream) {
		return nil, errors.New("Not enough speech to identify the speaker.")
	}
	vector := m.extractor.Compute(stream)
	if !validVectors([][]float32{vector}) {
		return nil, errors.New("The voiceprint model returned an invalid embedding.")
	}
	return vector, nil
}

func (m *Manager) Embed(samples []float32, rate int) (vector []float32, err error) {
	defer func() {
		if recover() != nil {
			vector, err = nil, errors.New("Voiceprint extraction failed.")
		}
	}()
	if m == nil {
		return nil, errors.New("Voiceprint manager is unavailable.")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if rate != voiceSampleRate || len(samples) < voiceSampleRate {
		return nil, errors.New("At least one second of 16 kHz speech is required.")
	}
	return m.embedLocked(samples, rate)
}

func (m *Manager) Verify(embeddings [][]float32, samples []float32, rate int) (matched bool, err error) {
	defer func() {
		if recover() != nil {
			matched, err = false, errors.New("Voiceprint verification failed.")
		}
	}()
	if m == nil || !validVectors(embeddings) {
		return false, errors.New("A valid voiceprint is required.")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	vector, err := m.embedLocked(samples, rate)
	if err != nil {
		return false, err
	}
	manager := sherpa.NewSpeakerEmbeddingManager(embeddingDimension)
	if manager == nil {
		return false, errors.New("Failed to create a speaker verifier.")
	}
	defer sherpa.DeleteSpeakerEmbeddingManager(manager)
	if !manager.RegisterV("me", embeddings) {
		return false, errors.New("Failed to register the voiceprint.")
	}
	return manager.Verify("me", vector, matchThreshold), nil
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.profileMu.Lock()
	m.closed.Store(true)
	m.runtime.system = nil
	m.revision++
	if m.profile != nil {
		m.profile.Revision = m.revision
	}
	m.profileMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.extractor != nil {
		sherpa.DeleteSpeakerEmbeddingExtractor(m.extractor)
		m.extractor = nil
	}
}
