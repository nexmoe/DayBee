package voiceprint

import (
	"errors"
	"math"
)

const (
	voiceSampleRate  = 16000
	minWindowSamples = 12800 // 0.8 s
	windowSamples    = 24000 // 1.5 s
	hopSamples       = 16000 // 1.0 s
)

// WindowScore is one sliding window of a microphone chunk.
type WindowScore struct {
	Start, End int // sample offsets; End is exclusive
	Score      float32
	Embedding  []float32
}

// Reference is system playback aligned to the same span and rate as the mic samples.
// A nil reference skips echo rejection. Complete is true only when every sample
// was actually observed; zero-filled gaps are not a confirmed observation.
type Reference struct {
	Samples  []float32
	Complete bool
}

// EchoWindows reports which mic windows are loudspeaker echo.
// Nil skips echo rejection. echo.go may assign this from init.
// Match calls it without the extractor lock, so the hook may embed.
var EchoWindows func(m *Manager, mic []float32, ref *Reference, wins []WindowScore, rate int) []bool

func Cosine(a, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// ProfileScore is max(cosine to the centroid, mean of the top-3 cosines).
func ProfileScore(p *Profile, v []float32) float32 {
	if p == nil || len(v) == 0 {
		return 0
	}
	centroid := make([]float64, len(v))
	cosines := make([]float32, 0, len(p.Embeddings))
	for _, embedding := range p.Embeddings {
		if len(embedding) != len(v) {
			continue
		}
		for i, value := range embedding {
			centroid[i] += float64(value)
		}
		cosines = append(cosines, Cosine(embedding, v))
	}
	if len(cosines) == 0 {
		return 0
	}
	center := make([]float32, len(v))
	scale := float64(len(cosines))
	for i := range center {
		center[i] = float32(centroid[i] / scale)
	}
	top := meanTop(cosines, 3)
	if centerScore := Cosine(center, v); centerScore > top {
		return centerScore
	}
	return top
}

func meanTop(values []float32, k int) float32 {
	if k > len(values) {
		k = len(values)
	}
	best := append([]float32(nil), values...)
	for i := 0; i < k; i++ {
		maxAt := i
		for j := i + 1; j < len(best); j++ {
			if best[j] > best[maxAt] {
				maxAt = j
			}
		}
		best[i], best[maxAt] = best[maxAt], best[i]
	}
	var sum float64
	for i := 0; i < k; i++ {
		sum += float64(best[i])
	}
	return float32(sum / float64(k))
}

func windowSpans(n, rate int) ([][2]int, error) {
	if rate != voiceSampleRate {
		return nil, errors.New("Voiceprint samples must be 16 kHz.")
	}
	if n < minWindowSamples {
		return nil, errors.New("At least 0.8 seconds of 16 kHz speech is required.")
	}
	if n <= windowSamples {
		return [][2]int{{0, n}}, nil
	}
	spans := make([][2]int, 0, 1+(n-windowSamples)/hopSamples)
	for start := 0; start+windowSamples <= n; start += hopSamples {
		spans = append(spans, [2]int{start, start + windowSamples})
	}
	return spans, nil
}

// EmbedWindows slides a 1.5 s window with a 1.0 s hop.
// Audio of 0.8–1.5 s is one window of the whole buffer. Shorter audio is an error.
func (m *Manager) EmbedWindows(samples []float32, rate int) (embs [][]float32, starts []int, err error) {
	defer func() {
		if recover() != nil {
			embs, starts, err = nil, nil, errors.New("Voiceprint extraction failed.")
		}
	}()
	if m == nil {
		return nil, nil, errors.New("Voiceprint manager is unavailable.")
	}
	spans, err := windowSpans(len(samples), rate)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	embs = make([][]float32, len(spans))
	starts = make([]int, len(spans))
	for i, span := range spans {
		vector, embedErr := m.embedLocked(samples[span[0]:span[1]], rate)
		if embedErr != nil {
			return nil, nil, embedErr
		}
		embs[i] = vector
		starts[i] = span[0]
	}
	return embs, starts, nil
}

func (m *Manager) Score(p *Profile, samples []float32, rate int) ([]WindowScore, error) {
	if p == nil || len(p.Embeddings) == 0 {
		return nil, errors.New("A valid voiceprint is required.")
	}
	embs, starts, err := m.EmbedWindows(samples, rate)
	if err != nil {
		return nil, err
	}
	spans, err := windowSpans(len(samples), rate)
	if err != nil || len(spans) != len(embs) {
		return nil, errors.New("Voiceprint windows did not match their embeddings.")
	}
	wins := make([]WindowScore, len(embs))
	for i := range embs {
		wins[i] = WindowScore{Start: starts[i], End: spans[i][1], Score: ProfileScore(p, embs[i]), Embedding: embs[i]}
	}
	return wins, nil
}

func (m *Manager) Model() string { return ModelName }

// Match scores the chunk and applies the decision rule. Errors only drop the badge.
func (m *Manager) Match(p *Profile, samples []float32, rate int, ref *Reference) (identity string, score float32) {
	defer func() {
		if recover() != nil {
			identity, score = "", 0
		}
	}()
	if m == nil || p == nil {
		return "", 0
	}
	wins, err := m.Score(p, samples, rate)
	if err != nil || len(wins) == 0 {
		return "", 0
	}
	var echo []bool
	if ref != nil && EchoWindows != nil {
		echo = EchoWindows(m, samples, ref, wins, rate)
	}
	return chunkLabel(p, wins, echo)
}

// chunkLabel is the public Match result. Version 3 uses the three-state decision,
// then Match keeps only "me" so older callers still see an empty string otherwise.
func chunkLabel(profile *Profile, windows []WindowScore, echo []bool) (string, float32) {
	if profile != nil && profile.Version >= calibratedVersion {
		identity, score := DecideProfile(profile, DecisionScores(profile, windows), echo)
		if identity != "me" {
			return "", score
		}
		return "me", score
	}
	scores := make([]float32, len(windows))
	for i, window := range windows {
		scores[i] = window.Score
	}
	model := ""
	if profile != nil {
		model = profile.Model
	}
	return Decide(scores, echo, model)
}
