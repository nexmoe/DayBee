package voiceprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// TestVoiceprintOfflineEval sweeps public 16 kHz speech when both
// DAYBEE_VOICEPRINT_EVAL_DIR and DAYBEE_VOICEPRINT_EVAL_MODEL are set.
// DAYBEE_VOICEPRINT_EVAL_OUT, when set, receives one JSON object of rates and timings.
//
// Enrollment is up to 24 s in 1.5 s windows with a 0.5 s hop. The window score is
// ProfileScore. The chunk score is the mean of the 3-point median-smoothed window
// scores. Echo trials are other speakers after evalEchoResidue.
func TestVoiceprintOfflineEval(t *testing.T) {
	dir := os.Getenv("DAYBEE_VOICEPRINT_EVAL_DIR")
	model := os.Getenv("DAYBEE_VOICEPRINT_EVAL_MODEL")
	if dir == "" || model == "" {
		t.Skip("Set DAYBEE_VOICEPRINT_EVAL_DIR and DAYBEE_VOICEPRINT_EVAL_MODEL")
	}
	speakers, err := evalLoadSpeakers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) < 10 {
		t.Fatalf("need at least 10 speakers with held-out speech, got %d", len(speakers))
	}
	extractor, err := evalOpenExtractor(model)
	if err != nil {
		t.Fatal(err)
	}
	defer extractor.close()
	ms, err := extractor.bench(speakers[0].enroll[:windowSamples])
	if err != nil {
		t.Fatal(err)
	}
	extractor.n = 0
	fmt.Printf("voiceprint eval: %s %.2f ms/window\n", filepath.Base(model), ms)
	for i := range speakers {
		if err := extractor.enroll(&speakers[i]); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("voiceprint eval: embedded %d/%d\n", i+1, len(speakers))
	}
	trials := evalBuildTrials(func(source, kind int) int {
		if kind == evalKindEcho {
			return len(speakers[source].echo)
		}
		return len(speakers[source].test)
	}, len(speakers))
	scores := make([]evalScoreSet, len(evalBuckets()))
	var echoAll []float64
	for _, trial := range trials {
		pcm, cache := speakers[trial.source].buffer(trial.kind)
		score, ok := extractor.scoreCrop(pcm, cache, trial.start, trial.length, speakers[trial.enrolled].profile)
		if !ok || math.IsNaN(score) {
			continue
		}
		switch trial.kind {
		case evalKindTarget:
			scores[trial.bucket].target = append(scores[trial.bucket].target, score)
		case evalKindOther:
			scores[trial.bucket].other = append(scores[trial.bucket].other, score)
		case evalKindEcho:
			scores[trial.bucket].echo = append(scores[trial.bucket].echo, score)
			echoAll = append(echoAll, score)
		}
	}
	report := evalReportFrom(filepath.Base(model), extractor.dim, ms, extractor.n, speakers, scores, echoAll)
	evalPrint(report)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("EVAL_JSON %s\n", encoded)
	if out := os.Getenv("DAYBEE_VOICEPRINT_EVAL_OUT"); out != "" {
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		pretty, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, append(pretty, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type evalSpeaker struct {
	uttN             int
	enroll           []float32
	test             []float32
	echo             []float32
	profile          *Profile
	testEmb, echoEmb map[[2]int][]float32
}

func (s evalSpeaker) buffer(kind int) ([]float32, map[[2]int][]float32) {
	if kind == evalKindEcho {
		return s.echo, s.echoEmb
	}
	return s.test, s.testEmb
}

type evalScoreSet struct {
	target, other, echo []float64
}

type evalExtractor struct {
	impl *sherpa.SpeakerEmbeddingExtractor
	dim  int
	n    int
}

func evalOpenExtractor(model string) (*evalExtractor, error) {
	info, err := os.Stat(model)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, errors.New("voiceprint eval model is missing")
	}
	impl := sherpa.NewSpeakerEmbeddingExtractor(&sherpa.SpeakerEmbeddingExtractorConfig{
		Model: model, NumThreads: 2, Provider: "cpu",
	})
	if impl == nil {
		return nil, errors.New("failed to open the voiceprint model")
	}
	if impl.Dim() <= 0 {
		sherpa.DeleteSpeakerEmbeddingExtractor(impl)
		return nil, errors.New("voiceprint model returned no embedding dimension")
	}
	return &evalExtractor{impl: impl, dim: impl.Dim()}, nil
}

func (e *evalExtractor) close() {
	if e != nil && e.impl != nil {
		sherpa.DeleteSpeakerEmbeddingExtractor(e.impl)
		e.impl = nil
	}
}

func (e *evalExtractor) embed(samples []float32) (vec []float32, err error) {
	defer func() {
		if recover() != nil {
			vec, err = nil, errors.New("embedding failed")
		}
	}()
	if len(samples) < minWindowSamples {
		return nil, errors.New("window is shorter than 0.8 s")
	}
	stream := e.impl.CreateStream()
	if stream == nil {
		return nil, errors.New("failed to create a voiceprint stream")
	}
	defer sherpa.DeleteOnlineStream(stream)
	stream.AcceptWaveform(voiceSampleRate, samples)
	stream.InputFinished()
	if !e.impl.IsReady(stream) {
		return nil, errors.New("not enough speech to embed")
	}
	vec = e.impl.Compute(stream)
	if !evalFiniteVector(vec, e.dim) {
		return nil, errors.New("invalid embedding")
	}
	e.n++
	return vec, nil
}

func (e *evalExtractor) bench(samples []float32) (float64, error) {
	for i := 0; i < 5; i++ {
		if _, err := e.embed(samples); err != nil {
			return 0, err
		}
	}
	const repeats = 30
	start := time.Now()
	for i := 0; i < repeats; i++ {
		if _, err := e.embed(samples); err != nil {
			return 0, err
		}
	}
	return float64(time.Since(start).Microseconds()) / 1000 / repeats, nil
}

func (e *evalExtractor) enroll(speaker *evalSpeaker) error {
	profile, err := e.vectors(speaker.enroll, nil, true)
	if err != nil {
		return err
	}
	if len(profile) < 3 {
		return errors.New("enrollment produced fewer than 3 windows")
	}
	speaker.profile = &Profile{Embeddings: profile}
	if _, err = e.vectors(speaker.test, speaker.testEmb, false); err != nil {
		return err
	}
	_, err = e.vectors(speaker.echo, speaker.echoEmb, false)
	return err
}

func (e *evalExtractor) vectors(pcm []float32, cache map[[2]int][]float32, enroll bool) ([][]float32, error) {
	if len(pcm) < minWindowSamples {
		return nil, nil
	}
	spans, err := windowSpans(len(pcm), voiceSampleRate)
	if err != nil {
		return nil, err
	}
	var vecs [][]float32
	for _, span := range spans {
		if enroll && span[1]-span[0] != windowSamples {
			continue
		}
		key := [2]int{span[0], span[1]}
		vec, ok := cache[key]
		if !ok {
			vec, err = e.embed(pcm[span[0]:span[1]])
			if err != nil {
				continue
			}
			if cache != nil {
				cache[key] = vec
			}
		}
		vecs = append(vecs, vec)
	}
	return vecs, nil
}

func (e *evalExtractor) scoreCrop(pcm []float32, cache map[[2]int][]float32, start, length int, profile *Profile) (float64, bool) {
	if profile == nil || start < 0 || length < minWindowSamples || start+length > len(pcm) {
		return 0, false
	}
	spans, err := windowSpans(length, voiceSampleRate)
	if err != nil || len(spans) == 0 {
		return 0, false
	}
	scores := make([]float32, 0, len(spans))
	for _, span := range spans {
		a, b := start+span[0], start+span[1]
		if b > len(pcm) {
			return 0, false
		}
		key := [2]int{a, b}
		vec, ok := cache[key]
		if !ok {
			var embedErr error
			vec, embedErr = e.embed(pcm[a:b])
			if embedErr != nil {
				return 0, false
			}
			if cache != nil {
				cache[key] = vec
			}
		}
		scores = append(scores, ProfileScore(profile, vec))
	}
	return evalChunkScore(scores), true
}

func evalLoadSpeakers(dir string) ([]evalSpeaker, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	grouped := map[string][]evalUtt{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id, index, ok := evalParseUtt(entry.Name())
		if !ok {
			continue
		}
		samples, err := evalReadPCM(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if len(samples) == 0 {
			continue
		}
		grouped[id] = append(grouped[id], evalUtt{index: index, samples: samples})
	}
	ids := make([]string, 0, len(grouped))
	for id := range grouped {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	speakers := make([]evalSpeaker, 0, len(ids))
	for _, id := range ids {
		utts := grouped[id]
		sort.Slice(utts, func(i, j int) bool { return utts[i].index < utts[j].index })
		enroll, test, from, ok := evalSplitEnroll(utts)
		if !ok {
			continue
		}
		echo := evalEchoFrom(id, utts[from:])
		if !evalFinitePCM(echo) {
			continue
		}
		speakers = append(speakers, evalSpeaker{
			uttN: len(utts), enroll: enroll, test: test, echo: echo,
			testEmb: map[[2]int][]float32{}, echoEmb: map[[2]int][]float32{},
		})
	}
	return speakers, nil
}

func evalEchoFrom(id string, utts []evalUtt) []float32 {
	parts := make([][]float32, len(utts))
	for i, utt := range utts {
		parts[i] = evalEchoResidue(evalRNG(id, utt.index), utt.samples)
	}
	return evalConcatPCM(parts, evalGapSamples)
}

func evalParseUtt(name string) (string, int, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	cut := strings.LastIndex(base, "_")
	if cut <= 0 {
		return "", 0, false
	}
	index, err := strconv.Atoi(base[cut+1:])
	if err != nil || index < 0 {
		return "", 0, false
	}
	return base[:cut], index, true
}

func evalReadPCM(path string) ([]float32, error) {
	wave := sherpa.ReadWave(path)
	if wave == nil {
		return nil, fmt.Errorf("unreadable wav %s", filepath.Base(path))
	}
	if wave.SampleRate != voiceSampleRate {
		return nil, fmt.Errorf("%s is %d Hz", filepath.Base(path), wave.SampleRate)
	}
	if len(wave.Samples) < voiceSampleRate/2 {
		return nil, nil
	}
	return wave.Samples, nil
}

func evalFinitePCM(samples []float32) bool {
	if len(samples) == 0 {
		return false
	}
	for _, sample := range samples {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return false
		}
	}
	return true
}

func evalFiniteVector(vec []float32, dim int) bool {
	if len(vec) != dim {
		return false
	}
	var norm float64
	for _, value := range vec {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		norm += float64(value) * float64(value)
	}
	return norm > 0
}

type evalReport struct {
	Model         string             `json:"model"`
	Dim           int                `json:"dim"`
	Speakers      int                `json:"speakers"`
	Utterances    int                `json:"utterances"`
	MeanEnrollSec float64            `json:"meanEnrollSec"`
	MeanTestSec   float64            `json:"meanTestSec"`
	Embeddings    int                `json:"embeddings"`
	MsPerWindow   float64            `json:"msPerWindow"`
	RTF           float64            `json:"rtf"`
	NumThreads    int                `json:"numThreads"`
	Buckets       []evalBucketReport `json:"buckets"`
	EchoAll       evalClassReport    `json:"echoAll"`
	TMe           float64            `json:"tMe"`
	FRAtTMe       float64            `json:"frAtTMe1to2s"`
	FAOtherAtTMe  float64            `json:"faOtherAtTMe1to2s"`
	FAEchoAtTMe   float64            `json:"faEchoAtTMe"`
}

type evalBucketReport struct {
	Bucket            string  `json:"bucket"`
	NTarget           int     `json:"nTarget"`
	NOther            int     `json:"nOther"`
	NEcho             int     `json:"nEcho"`
	EEROther          float64 `json:"eerOther"`
	ThresholdFA1Other float64 `json:"thresholdFA1Other"`
	FRAtFA1Other      float64 `json:"frAtFA1Other"`
	EEREcho           float64 `json:"eerEcho"`
	ThresholdFA1Echo  float64 `json:"thresholdFA1Echo"`
	FRAtFA1Echo       float64 `json:"frAtFA1Echo"`
}

type evalClassReport struct {
	N            int     `json:"n"`
	EER          float64 `json:"eer"`
	ThresholdFA1 float64 `json:"thresholdFA1"`
	FRAtFA1      float64 `json:"frAtFA1"`
}

func evalReportFrom(model string, dim int, ms float64, embeddings int, speakers []evalSpeaker, scores []evalScoreSet, echoAll []float64) evalReport {
	var enrollSec, testSec float64
	var utterances int
	for _, speaker := range speakers {
		utterances += speaker.uttN
		enrollSec += float64(len(speaker.enroll)) / voiceSampleRate
		testSec += float64(len(speaker.test)) / voiceSampleRate
	}
	n := float64(len(speakers))
	report := evalReport{
		Model: model, Dim: dim, Speakers: len(speakers), Utterances: utterances,
		MeanEnrollSec: enrollSec / n, MeanTestSec: testSec / n,
		Embeddings: embeddings, MsPerWindow: ms, RTF: ms / 1500, NumThreads: 2,
	}
	names := evalBuckets()
	echoSets := make([][]float64, len(scores))
	for i, set := range scores {
		eerO, thrO, frO := evalSummarize(set.target, set.other)
		eerE, thrE, frE := evalSummarize(set.target, set.echo)
		report.Buckets = append(report.Buckets, evalBucketReport{
			Bucket: names[i].name, NTarget: len(set.target), NOther: len(set.other), NEcho: len(set.echo),
			EEROther: eerO, ThresholdFA1Other: thrO, FRAtFA1Other: frO,
			EEREcho: eerE, ThresholdFA1Echo: thrE, FRAtFA1Echo: frE,
		})
		echoSets[i] = set.echo
	}
	var targetsAll []float64
	for _, set := range scores {
		targetsAll = append(targetsAll, set.target...)
	}
	eerA, thrA, frA := evalSummarize(targetsAll, echoAll)
	report.EchoAll = evalClassReport{N: len(echoAll), EER: eerA, ThresholdFA1: thrA, FRAtFA1: frA}
	report.TMe = report.Buckets[0].ThresholdFA1Other
	for _, set := range echoSets {
		if threshold, _ := evalThresholdAtFA(set, 0.01); threshold > report.TMe {
			report.TMe = threshold
		}
	}
	if thrA > report.TMe {
		report.TMe = thrA
	}
	report.FRAtTMe = evalFalseReject(scores[0].target, report.TMe)
	report.FAOtherAtTMe = evalFalseAccept(scores[0].other, report.TMe)
	report.FAEchoAtTMe = evalFalseAccept(echoAll, report.TMe)
	return report
}

func evalSummarize(target, nontarget []float64) (eer, threshold, fr float64) {
	return evalEER(target, nontarget), evalFirst(evalThresholdAtFA(nontarget, 0.01)), evalFalseReject(target, evalFirst(evalThresholdAtFA(nontarget, 0.01)))
}

func evalFirst(value, _ float64) float64 { return value }

func evalPrint(report evalReport) {
	fmt.Printf("model %s dim %d speakers %d utterances %d meanEnroll %.2fs meanTest %.2fs ms/window %.2f rtf %.4f\n",
		report.Model, report.Dim, report.Speakers, report.Utterances, report.MeanEnrollSec, report.MeanTestSec, report.MsPerWindow, report.RTF)
	fmt.Printf("%-6s %7s %7s %7s %8s %8s %8s %8s %8s %8s\n",
		"bucket", "nTarget", "nOther", "nEcho", "eerOth", "thrOth", "frOth", "eerEcho", "thrEcho", "frEcho")
	for _, bucket := range report.Buckets {
		fmt.Printf("%-6s %7d %7d %7d %8.4f %8.4f %8.4f %8.4f %8.4f %8.4f\n",
			bucket.Bucket, bucket.NTarget, bucket.NOther, bucket.NEcho,
			bucket.EEROther, bucket.ThresholdFA1Other, bucket.FRAtFA1Other,
			bucket.EEREcho, bucket.ThresholdFA1Echo, bucket.FRAtFA1Echo)
	}
	fmt.Printf("echoAll n %d eer %.4f thrFA1 %.4f fr %.4f\n", report.EchoAll.N, report.EchoAll.EER, report.EchoAll.ThresholdFA1, report.EchoAll.FRAtFA1)
	fmt.Printf("tMe %.4f frAtTMe %.4f faOther %.4f faEcho %.4f\n", report.TMe, report.FRAtTMe, report.FAOtherAtTMe, report.FAEchoAtTMe)
}
