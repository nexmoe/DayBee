// Command gen-cohort builds the bundled ERes2NetV2 impostor cohort.
// It reads AISHELL-3 test WAVs, keeps 32 windows from each of 9 speakers,
// and refuses the four held-out speakers entirely.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

const (
	modelName         = "3dspeaker_speech_eres2netv2_sv_zh-cn_16k-common.onnx"
	sampleRate        = 16000
	embeddingDim      = 192
	minSamples        = 12800 // 0.8 s
	windowSamples     = 24000 // 1.5 s
	hopSamples        = 8000  // 0.5 s
	windowsPerSpeaker = 32
	datasetURL        = "https://openslr.org/93/"
	selectionRule     = "sort by file name then sample offset; keep i*(n-1)/(k-1) for i=0..31"
	windowRule        = "an utterance of 0.8s to 1.5s is one whole-utterance window; longer audio uses 1.5s windows with a 0.5s hop that stay inside the utterance"
)

var cohortSpeakers = []string{
	"SSB0193", "SSB0407", "SSB0686", "SSB0702", "SSB0711",
	"SSB0716", "SSB0717", "SSB0736", "SSB0749",
}

var heldOutSpeakers = []string{"SSB0434", "SSB0544", "SSB0693", "SSB0700"}

type span struct {
	file    string
	path    string
	offset  int
	samples []float32
	fileSHA string
}

type metaHeader struct {
	Format            string   `json:"format"`
	Model             string   `json:"model"`
	ModelSHA256       string   `json:"modelSHA256"`
	Dataset           string   `json:"dataset"`
	DatasetURL        string   `json:"datasetURL"`
	License           string   `json:"license"`
	Dim               int      `json:"dim"`
	Speakers          []string `json:"speakers"`
	VectorsPerSpeaker int      `json:"vectorsPerSpeaker"`
	Vectors           int      `json:"vectors"`
	HeldOut           []string `json:"heldOut"`
	SampleRate        int      `json:"sampleRate"`
	WindowSeconds     float64  `json:"windowSeconds"`
	MinWindowSeconds  float64  `json:"minWindowSeconds"`
	HopSeconds        float64  `json:"hopSeconds"`
	Selection         string   `json:"selection"`
	WindowRule        string   `json:"windowRule"`
	BlobSHA256        string   `json:"blobSHA256"`
}

type windowRec struct {
	Speaker      string `json:"speaker"`
	File         string `json:"file"`
	Offset       int    `json:"offset"`
	Samples      int    `json:"samples"`
	FileSHA256   string `json:"fileSHA256"`
	WindowSHA256 string `json:"windowSHA256"`
}

func main() {
	wavDir := flag.String("wav", "", "directory of 16 kHz AISHELL-3 WAVs")
	modelPath := flag.String("model", "", "ERes2NetV2 ONNX path")
	outDir := flag.String("out", "", "directory for cohort_eres2netv2.f32 and cohort_meta.json")
	flag.Parse()
	if err := run(*wavDir, *modelPath, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "gen-cohort: %v\n", err)
		os.Exit(1)
	}
}

func run(wavDir, modelPath, outDir string) error {
	if wavDir == "" || modelPath == "" || outDir == "" {
		return errors.New("wav, model, and out are required")
	}
	if filepath.Base(modelPath) != modelName {
		return fmt.Errorf("model file must be %s", modelName)
	}
	modelSHA, err := fileSHA256(modelPath)
	if err != nil {
		return err
	}
	picked, err := selectWindows(wavDir)
	if err != nil {
		return err
	}
	vectors, err := embedAll(modelPath, picked)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	blobPath := filepath.Join(outDir, "cohort_eres2netv2.f32")
	blobSHA, err := writeBlob(blobPath, vectors)
	if err != nil {
		return err
	}
	recs := make([]windowRec, len(picked))
	for i, sp := range picked {
		sum, err := sha256Samples(sp.samples)
		if err != nil {
			return err
		}
		recs[i] = windowRec{
			Speaker: speakerOf(sp.file), File: sp.file, Offset: sp.offset,
			Samples: len(sp.samples), FileSHA256: sp.fileSHA, WindowSHA256: sum,
		}
	}
	header := metaHeader{
		Format: "float32-le", Model: modelName, ModelSHA256: modelSHA,
		Dataset: "AISHELL-3", DatasetURL: datasetURL, License: "Apache-2.0",
		Dim: embeddingDim, Speakers: append([]string(nil), cohortSpeakers...),
		VectorsPerSpeaker: windowsPerSpeaker, Vectors: len(recs),
		HeldOut: append([]string(nil), heldOutSpeakers...), SampleRate: sampleRate,
		WindowSeconds: 1.5, MinWindowSeconds: 0.8, HopSeconds: 0.5,
		Selection: selectionRule, WindowRule: windowRule, BlobSHA256: blobSHA,
	}
	metaPath := filepath.Join(outDir, "cohort_meta.json")
	if err := writeMeta(metaPath, header, recs); err != nil {
		return err
	}
	fmt.Printf("wrote %d vectors dim %d\nblob %s\nmeta %s\n", len(vectors), embeddingDim, blobSHA, metaPath)
	return nil
}

func selectWindows(wavDir string) ([]span, error) {
	grouped, err := groupFiles(wavDir)
	if err != nil {
		return nil, err
	}
	var picked []span
	for _, speaker := range cohortSpeakers {
		files := grouped[speaker]
		candidates, err := candidateWindows(files)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", speaker, err)
		}
		chosen, err := pickEven(candidates, windowsPerSpeaker)
		if err != nil {
			return nil, fmt.Errorf("%s: %d candidates: %w", speaker, len(candidates), err)
		}
		fmt.Printf("%s candidates %d selected %d\n", speaker, len(candidates), len(chosen))
		picked = append(picked, chosen...)
	}
	if len(picked) != len(cohortSpeakers)*windowsPerSpeaker {
		return nil, fmt.Errorf("vector count %d", len(picked))
	}
	return picked, nil
}

func groupFiles(wavDir string) (map[string][]string, error) {
	entries, err := os.ReadDir(wavDir)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, id := range cohortSpeakers {
		allowed[id] = true
	}
	for _, id := range heldOutSpeakers {
		allowed[id] = true
	}
	grouped := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".wav") {
			continue
		}
		speaker, ok := speakerID(entry.Name())
		if !ok {
			return nil, fmt.Errorf("unparsed wav name %s", entry.Name())
		}
		if !allowed[speaker] {
			return nil, fmt.Errorf("unexpected speaker %s", speaker)
		}
		if heldOut(speaker) {
			continue
		}
		grouped[speaker] = append(grouped[speaker], filepath.Join(wavDir, entry.Name()))
	}
	for _, speaker := range cohortSpeakers {
		if len(grouped[speaker]) == 0 {
			return nil, fmt.Errorf("missing speaker %s", speaker)
		}
		sort.Strings(grouped[speaker])
	}
	return grouped, nil
}

func candidateWindows(files []string) ([]span, error) {
	var out []span
	for _, path := range files {
		samples, fileSHA, err := readWav(path)
		if err != nil {
			return nil, err
		}
		for _, sp := range utteranceSpans(len(samples)) {
			out = append(out, span{
				file: filepath.Base(path), path: path, offset: sp[0],
				samples: append([]float32(nil), samples[sp[0]:sp[1]]...), fileSHA: fileSHA,
			})
		}
	}
	return out, nil
}

func utteranceSpans(n int) [][2]int {
	if n < minSamples {
		return nil
	}
	if n <= windowSamples {
		return [][2]int{{0, n}}
	}
	var spans [][2]int
	for start := 0; start+windowSamples <= n; start += hopSamples {
		spans = append(spans, [2]int{start, start + windowSamples})
	}
	return spans
}

func pickEven(in []span, k int) ([]span, error) {
	n := len(in)
	if k < 2 || n < k {
		return nil, fmt.Errorf("need at least %d windows, have %d", k, n)
	}
	out := make([]span, k)
	seen := make(map[int]bool, k)
	for i := 0; i < k; i++ {
		idx := i * (n - 1) / (k - 1)
		if seen[idx] {
			return nil, fmt.Errorf("selection collided at %d", idx)
		}
		seen[idx] = true
		out[i] = in[idx]
	}
	return out, nil
}

func embedAll(modelPath string, windows []span) ([][]float32, error) {
	extractor := sherpa.NewSpeakerEmbeddingExtractor(&sherpa.SpeakerEmbeddingExtractorConfig{
		Model: modelPath, NumThreads: 2, Provider: "cpu",
	})
	if extractor == nil {
		return nil, errors.New("failed to open the voiceprint model")
	}
	defer sherpa.DeleteSpeakerEmbeddingExtractor(extractor)
	if extractor.Dim() != embeddingDim {
		return nil, fmt.Errorf("model dim %d", extractor.Dim())
	}
	out := make([][]float32, len(windows))
	for i, sp := range windows {
		vector, err := embedOne(extractor, sp.samples)
		if err != nil {
			return nil, fmt.Errorf("%s@%d: %w", sp.file, sp.offset, err)
		}
		out[i] = vector
		if (i+1)%32 == 0 {
			fmt.Printf("embedded %d/%d\n", i+1, len(windows))
		}
	}
	return out, nil
}

func embedOne(extractor *sherpa.SpeakerEmbeddingExtractor, samples []float32) ([]float32, error) {
	stream := extractor.CreateStream()
	if stream == nil {
		return nil, errors.New("failed to create a stream")
	}
	defer sherpa.DeleteOnlineStream(stream)
	stream.AcceptWaveform(sampleRate, samples)
	stream.InputFinished()
	if !extractor.IsReady(stream) {
		return nil, errors.New("model rejected the window")
	}
	vector := extractor.Compute(stream)
	if err := finiteNorm(vector); err != nil {
		return nil, err
	}
	return vector, nil
}

func finiteNorm(vector []float32) error {
	if len(vector) != embeddingDim {
		return fmt.Errorf("dim %d", len(vector))
	}
	var norm float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("non-finite embedding")
		}
		norm += float64(value) * float64(value)
	}
	if !(norm > 0) || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return fmt.Errorf("embedding norm %g", math.Sqrt(norm))
	}
	return nil
}

func writeBlob(path string, vectors [][]float32) (string, error) {
	buf := make([]byte, 0, len(vectors)*embeddingDim*4)
	var le [4]byte
	for _, vector := range vectors {
		if len(vector) != embeddingDim {
			return "", errors.New("blob vector dim")
		}
		for _, value := range vector {
			binary.LittleEndian.PutUint32(le[:], math.Float32bits(value))
			buf = append(buf, le[:]...)
		}
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

func writeMeta(path string, header metaHeader, windows []windowRec) error {
	body, err := json.MarshalIndent(header, "", "  ")
	if err != nil {
		return err
	}
	if !strings.HasSuffix(string(body), "\n}") {
		return errors.New("meta header shape")
	}
	var b strings.Builder
	b.Write(body[:len(body)-2])
	b.WriteString(",\n  \"windows\": [\n")
	for i, rec := range windows {
		line, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		b.WriteString("    ")
		b.Write(line)
		if i+1 != len(windows) {
			b.WriteString(",\n")
		} else {
			b.WriteByte('\n')
		}
	}
	b.WriteString("  ]\n}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readWav(path string) ([]float32, string, error) {
	fileSHA, err := fileSHA256(path)
	if err != nil {
		return nil, "", err
	}
	wave := sherpa.ReadWave(path)
	if wave == nil {
		return nil, "", fmt.Errorf("unreadable wav %s", filepath.Base(path))
	}
	if wave.SampleRate != sampleRate {
		return nil, "", fmt.Errorf("%s is %d Hz", filepath.Base(path), wave.SampleRate)
	}
	return wave.Samples, fileSHA, nil
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sha256Samples(samples []float32) (string, error) {
	buf := make([]byte, len(samples)*4)
	var le [4]byte
	for i, value := range samples {
		binary.LittleEndian.PutUint32(le[:], math.Float32bits(value))
		copy(buf[i*4:], le[:])
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

func speakerID(name string) (string, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	cut := strings.LastIndex(base, "_")
	if cut <= 0 || cut == len(base)-1 {
		return "", false
	}
	return base[:cut], true
}

func speakerOf(name string) string {
	id, _ := speakerID(name)
	return id
}

func heldOut(speaker string) bool {
	for _, id := range heldOutSpeakers {
		if id == speaker {
			return true
		}
	}
	return false
}
