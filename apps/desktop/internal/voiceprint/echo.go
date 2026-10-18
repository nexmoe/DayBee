package voiceprint

import "math"

const (
	echoFrameMS   = 10
	echoMaxLagMS  = 300
	echoSilenceDB = -50.0
	echoMargin    = float32(0.05)
	echoRhoMin    = float32(0.6)
	echoSlack     = float32(0.1)
)

func init() { EchoWindows = echoWindows }

// echoWindows marks microphone windows that are loudspeaker playback.
// A quiet system window is never echo. Otherwise the window is echo when the
// lagged system embedding is within 0.05 of the microphone score, or the
// envelope correlation is at least 0.6 and the microphone score is not confident.
func echoWindows(m *Manager, mic []float32, ref *Reference, wins []WindowScore, rate int) []bool {
	flags, _ := echoAnalysis(m, mic, ref, wins, rate)
	return flags
}

// echoAnalysis is the default echo pass. Vectors are the embeddings already
// computed for the lagged system audio, nil when that window is silent or failed.
func echoAnalysis(m *Manager, mic []float32, ref *Reference, wins []WindowScore, rate int) ([]bool, [][]float32) {
	out := make([]bool, len(wins))
	if ref == nil || len(ref.Samples) == 0 || rate <= 0 {
		return out, nil
	}
	frame := rate * echoFrameMS / 1000
	if frame < 1 {
		frame = 1
	}
	threshold := Threshold("")
	if m != nil {
		threshold = Threshold(m.Model())
	}
	vectors := make([][]float32, len(wins))
	for i, win := range wins {
		system := span(ref.Samples, win.Start, win.End)
		if rmsDBFS(system) <= echoSilenceDB {
			continue
		}
		micSpan := span(mic, win.Start, win.End)
		rho, lagFrames := envelopeXCorr(frameLogEnergy(micSpan, frame), frameLogEnergy(system, frame), echoMaxLagMS/echoFrameMS)
		shift := lagFrames * frame
		emb, score, ok := systemObservation(m, span(ref.Samples, win.Start-shift, win.End-shift), micSpan, win.Embedding, rate)
		out[i] = echoByScore(score, ok, rho, win.Score, threshold)
		if ok && validEmbedding(emb) {
			vectors[i] = emb
		}
	}
	return out, vectors
}

func echoByScore(system float32, have bool, rho, score, threshold float32) bool {
	if have && system >= score-echoMargin {
		return true
	}
	return rho >= echoRhoMin && score < threshold+echoSlack
}

func systemObservation(m *Manager, samples, micWindow, micEmb []float32, rate int) ([]float32, float32, bool) {
	if m == nil || len(samples) == 0 || len(micEmb) == 0 {
		return nil, 0, false
	}
	// The lagged system span is the microphone window already embedded.
	if sameSamples(samples, micWindow) {
		return append([]float32(nil), micEmb...), 1, true
	}
	embs, _, err := m.EmbedWindows(samples, rate)
	if err != nil || len(embs) == 0 {
		return nil, 0, false
	}
	emb := embs[len(embs)/2]
	if len(emb) == 0 || len(emb) != len(micEmb) {
		return nil, 0, false
	}
	return emb, Cosine(micEmb, emb), true
}

func sameSamples(a, b []float32) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func span(samples []float32, start, end int) []float32 {
	if start < 0 {
		start = 0
	}
	if end > len(samples) {
		end = len(samples)
	}
	if end <= start {
		return nil
	}
	return samples[start:end]
}

func rmsDBFS(samples []float32) float64 {
	if len(samples) == 0 {
		return math.Inf(-1)
	}
	var sum float64
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms <= 0 || math.IsNaN(rms) {
		return math.Inf(-1)
	}
	return 20 * math.Log10(rms)
}

// frameLogEnergy is the natural log of mean-square energy over fixed frames.
func frameLogEnergy(samples []float32, frame int) []float32 {
	if frame < 1 || len(samples) < frame {
		return nil
	}
	out := make([]float32, len(samples)/frame)
	for i := range out {
		var sum float64
		chunk := samples[i*frame : (i+1)*frame]
		for _, sample := range chunk {
			sum += float64(sample) * float64(sample)
		}
		out[i] = float32(math.Log(sum/float64(frame) + 1e-12))
	}
	return out
}

// envelopeXCorr is the maximum Pearson correlation of mic lagging ref by 0..maxLag frames.
func envelopeXCorr(mic, ref []float32, maxLag int) (float32, int) {
	n := len(mic)
	if len(ref) < n {
		n = len(ref)
	}
	if n < 2 {
		return 0, 0
	}
	if maxLag < 0 {
		maxLag = 0
	}
	if maxLag > n-2 {
		maxLag = n - 2
	}
	best, lag := float32(-2), 0
	for k := 0; k <= maxLag; k++ {
		overlap := n - k
		if corr := pearson(mic[k:k+overlap], ref[:overlap]); corr > best {
			best, lag = corr, k
		}
	}
	return best, lag
}

func pearson(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n < 2 {
		return 0
	}
	var meanA, meanB float64
	for i := 0; i < n; i++ {
		meanA += float64(a[i])
		meanB += float64(b[i])
	}
	meanA /= float64(n)
	meanB /= float64(n)
	var num, da, db float64
	for i := 0; i < n; i++ {
		xa, xb := float64(a[i])-meanA, float64(b[i])-meanB
		num += xa * xb
		da += xa * xa
		db += xb * xb
	}
	if da == 0 || db == 0 {
		return 0
	}
	return float32(num / math.Sqrt(da*db))
}
