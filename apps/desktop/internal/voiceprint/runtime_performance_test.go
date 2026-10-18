package voiceprint

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
	"github.com/nexmoe/eve/apps/desktop/internal/asr"
)

func perfReportPath() string {
	if path := os.Getenv("DAYBEE_VOICEPRINT_PERFORMANCE_OUT"); path != "" {
		return path
	}
	return "/tmp/voiceprint-runtime-performance-20261012.json"
}

func TestPerfSummary(t *testing.T) {
	mean, p50, p95 := perfSummary([]float64{1, 2, 3, 4, 5})
	if mean != 3 || p50 != 3 || p95 != 5 {
		t.Fatalf("summary mean=%v p50=%v p95=%v", mean, p50, p95)
	}
	if mean, p50, p95 = perfSummary(nil); mean != 0 || p50 != 0 || p95 != 0 {
		t.Fatal("empty summary")
	}
}

func perfSummary(values []float64) (mean, p50, p95 float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	var sum float64
	for _, value := range ordered {
		sum += value
	}
	return sum / float64(len(ordered)), ordered[(len(ordered)-1)/2], ordered[int(math.Ceil(0.95*float64(len(ordered))))-1]
}

// TestNativeRuntimePerformance measures fixed public audio. It does not record
// a microphone. Queue wait is not included: Decode is called on one recognizer.
func TestNativeRuntimePerformance(t *testing.T) {
	model := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_MODEL")
	wavDir := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_WAV")
	asrDir := os.Getenv("DAYBEE_TEST_MODEL_DIR")
	audioDir := os.Getenv("DAYBEE_TEST_AUDIO_DIR")
	if model == "" || wavDir == "" || asrDir == "" || audioDir == "" {
		t.Skip("Set DAYBEE_VOICEPRINT_PIPELINE_MODEL, DAYBEE_VOICEPRINT_PIPELINE_WAV, DAYBEE_TEST_MODEL_DIR, and DAYBEE_TEST_AUDIO_DIR")
	}
	report := perfReport{Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, ProductionVoiceprintThreads: 2,
		QueueWaitMeasured: false, HeapNote: "Go heap only. This is not native ONNX RSS.",
		Limitations: []string{
			"Decode is called on one production recognizer. ASR worker queue wait is not measured.",
			"Each clip is one VAD utterance. Confidence spans are measured, but the complete clip is decoded once, as in the recorder path.",
			"No real room was recorded. Silence and the gain-0.5 own_video reference are synthetic.",
			"Thread timings use a separate sherpa extractor. Production Manager stays at NumThreads 2.",
			"Each measured run starts from a cleared adaptation pool. A zero pool means the conservative gate did not fire, not that adaptation has no benefit.",
		}}
	for _, key := range []string{"DAYBEE_VOICEPRINT_PERFORMANCE_OUT", "DAYBEE_VOICEPRINT_PIPELINE_MODEL", "DAYBEE_VOICEPRINT_PIPELINE_WAV", "DAYBEE_TEST_MODEL_DIR", "DAYBEE_TEST_AUDIO_DIR"} {
		report.Env = append(report.Env, perfEnv{Key: key, Value: os.Getenv(key)})
	}
	speakers, err := runtimeEvalLoad(wavDir)
	if err != nil {
		t.Fatal(err)
	}
	owner := perfSpeaker(speakers, "SSB0544")
	if owner == nil || len(owner.Test) < windowSamples {
		t.Fatal("SSB0544 held-out audio is shorter than 1.5s")
	}
	report.Threads = perfThreads(t, model, owner.Test[:windowSamples])
	manager, err := New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	runtimeEvalEnroll(t, manager, owner)
	if owner.Profile == nil {
		t.Fatalf("SSB0544 was not enrolled: %s", owner.Error)
	}
	if err = manager.SaveProfile(owner.Profile); err != nil {
		t.Fatal(err)
	}
	report.Enrollment = perfEnrollmentOf(*owner)
	if !report.Enrollment.HeldOutExcluded {
		t.Fatal("held-out audio was mixed into enrollment")
	}
	if len(owner.Test) < 8*voiceSampleRate {
		t.Fatal("SSB0544 held-out audio is under 8s")
	}
	other := perfOther(speakers, owner.ID)
	if other == nil {
		t.Fatal("no second speaker has 4s of held-out speech")
	}
	me, meOK := runtimeEvalPrefix(owner.Test, 4*voiceSampleRate)
	them, themOK := runtimeEvalPrefix(other.Test, 4*voiceSampleRate)
	if !meOK || !themOK {
		t.Fatal("me-other-me needs 4s of held-out speech from two speakers")
	}
	chinese := perfRead(t, filepath.Join(audioDir, "chinese.wav"))
	sequence := append(append(append([]float32{}, me...), them...), me...)
	held := append([]float32(nil), owner.Test...)
	clips := []perfClip{
		{Name: "chinese", Samples: chinese, Ref: perfQuiet(len(chinese)), Reference: "complete-silence"},
		{Name: "me-other-me", Samples: sequence, Ref: perfQuiet(len(sequence)), Reference: "complete-silence"},
		{Name: "own_video", Samples: runtimeEvalScale(held, runtimeEvalOwnGain), Ref: runtimeEvalRef(held), Reference: "raw-held-out"},
		{Name: "clean_me_with_quiet_reference", Samples: held, Ref: perfQuiet(len(held)), Reference: "complete-silence"},
	}
	recognizer, err := (asr.Sherpa{}).NewRecognizer(asrDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recognizer.Close()
	for _, clip := range clips {
		report.Clips = append(report.Clips, perfMeasureClip(t, manager, recognizer, clip))
	}
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	report.HeapAlloc = mem.HeapAlloc
	report.HeapInuse = mem.HeapInuse
	report.HeapSys = mem.HeapSys
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(perfReportPath(), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Threads {
		if row.Repeats < 20 || row.MeanMs <= 0 || row.P95Ms < row.P50Ms {
			t.Fatalf("thread summary %+v", row)
		}
	}
	used := report.Enrollment.UsedGroupSeconds
	if len(used) != 3 || used[0] != 8 || used[1] != 8 || used[2] != 8 {
		t.Fatalf("used enrollment seconds %+v", used)
	}
	if len(report.Clips) != 4 {
		t.Fatalf("clips %d", len(report.Clips))
	}
	for _, clip := range report.Clips {
		if clip.InputSeconds < 8 || len(clip.Modes) != 3 {
			t.Fatalf("clip %+v", clip)
		}
		for _, mode := range clip.Modes {
			if len(mode.Runs) != 3 {
				t.Fatalf("%s %s runs %+v", clip.Name, mode.Mode, mode.Runs)
			}
			for _, run := range mode.Runs {
				if !run.CoverageComplete || run.DecodeCount < 1 || run.DecodeMs <= 0 || run.TotalMs < run.DecodeMs {
					t.Fatalf("%s %s %+v", clip.Name, mode.Mode, run)
				}
				if run.DecodeCount != 1 {
					t.Fatalf("%s cut an utterance at confidence boundaries: %+v", clip.Name, run)
				}
			}
		}
	}
}

type perfReport struct {
	Go                          string           `json:"go"`
	OS                          string           `json:"os"`
	Arch                        string           `json:"arch"`
	Env                         []perfEnv        `json:"env"`
	ProductionVoiceprintThreads int              `json:"productionVoiceprintThreads"`
	QueueWaitMeasured           bool             `json:"queueWaitMeasured"`
	HeapNote                    string           `json:"heapNote"`
	HeapAlloc                   uint64           `json:"heapAllocBytes"`
	HeapInuse                   uint64           `json:"heapInuseBytes"`
	HeapSys                     uint64           `json:"heapSysBytes"`
	Limitations                 []string         `json:"limitations"`
	Threads                     []perfThread     `json:"voiceprintThreads"`
	Enrollment                  perfEnrollment   `json:"enrollment"`
	Clips                       []perfClipResult `json:"clips"`
}

type perfEnv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type perfThread struct {
	Threads  int     `json:"threads"`
	WindowMs float64 `json:"windowMs"`
	Warmup   int     `json:"warmup"`
	Repeats  int     `json:"repeats"`
	MeanMs   float64 `json:"meanMs"`
	P50Ms    float64 `json:"p50Ms"`
	P95Ms    float64 `json:"p95Ms"`
}

type perfEnrollment struct {
	Speaker          string     `json:"speaker"`
	Split            string     `json:"split"`
	EnrollmentFiles  [][]string `json:"enrollmentFiles"`
	TestFiles        []string   `json:"testFiles"`
	HeldOutExcluded  bool       `json:"heldOutExcluded"`
	GroupSeconds     []float64  `json:"groupSeconds"`
	GroupSecondsNote string     `json:"groupSecondsNote"`
	UsedGroupSeconds []float64  `json:"usedGroupSeconds"`
	TestSeconds      float64    `json:"testSeconds"`
}

type perfClip struct {
	Name      string
	Samples   []float32
	Ref       *Reference
	Reference string
}

type perfClipResult struct {
	Name         string     `json:"name"`
	Reference    string     `json:"reference"`
	InputSeconds float64    `json:"inputSeconds"`
	Modes        []perfMode `json:"modes"`
}

type perfRun struct {
	VoiceprintMs     float64 `json:"voiceprintMs"`
	DecodeCount      int     `json:"decodeCount"`
	DecodeMs         float64 `json:"decodeMs"`
	TotalMs          float64 `json:"totalMs"`
	RTF              float64 `json:"rtf"`
	AdaptationCount  int     `json:"adaptationCount"`
	SystemCount      int     `json:"systemCount"`
	SpanCount        int     `json:"spanCount"`
	CoverageComplete bool    `json:"coverageComplete"`
	Error            string  `json:"error,omitempty"`
}

type perfMode struct {
	Mode             string    `json:"mode"`
	WarmupExcluded   bool      `json:"warmupExcluded"`
	Runs             []perfRun `json:"runs"`
	VoiceprintMeanMs float64   `json:"voiceprintMeanMs"`
	DecodeMeanMs     float64   `json:"decodeMeanMs"`
	TotalMeanMs      float64   `json:"totalMeanMs"`
	TotalP50Ms       float64   `json:"totalP50Ms"`
	TotalP95Ms       float64   `json:"totalP95Ms"`
	Note             string    `json:"note,omitempty"`
}

func perfSpeaker(speakers []runtimeEvalSpeaker, id string) *runtimeEvalSpeaker {
	for i := range speakers {
		if speakers[i].ID == id {
			return &speakers[i]
		}
	}
	return nil
}

func perfOther(speakers []runtimeEvalSpeaker, self string) *runtimeEvalSpeaker {
	for i := range speakers {
		if speakers[i].ID != self && len(speakers[i].Test) >= 4*voiceSampleRate {
			return &speakers[i]
		}
	}
	return nil
}

func perfEnrollmentOf(speaker runtimeEvalSpeaker) perfEnrollment {
	used := map[string]bool{}
	for _, group := range speaker.EnrollmentFiles {
		for _, name := range group {
			used[name] = true
		}
	}
	clean := true
	for _, name := range speaker.TestFiles {
		if used[name] {
			clean = false
		}
	}
	return perfEnrollment{Speaker: speaker.ID, Split: speaker.Split, EnrollmentFiles: speaker.EnrollmentFiles, TestFiles: speaker.TestFiles,
		HeldOutExcluded: clean && len(speaker.TestFiles) > 0, GroupSeconds: speaker.GroupSeconds,
		GroupSecondsNote: "groupSeconds is the available raw duration. usedGroupSeconds is what enrollment embedded.",
		UsedGroupSeconds: []float64{runtimeEvalGroupSec, runtimeEvalGroupSec, runtimeEvalGroupSec},
		TestSeconds:      float64(len(speaker.Test)) / voiceSampleRate}
}

func perfQuiet(n int) *Reference {
	return &Reference{Samples: make([]float32, n), Complete: true}
}

func perfRead(t *testing.T, path string) []float32 {
	t.Helper()
	samples, rate, err := (asr.Sherpa{}).ReadWave(path)
	if err != nil || rate != voiceSampleRate || len(samples) == 0 {
		t.Fatal(path, rate, err)
	}
	return samples
}

func perfThreads(t *testing.T, model string, window []float32) []perfThread {
	t.Helper()
	var rows []perfThread
	for _, threads := range []int{1, 2} {
		extractor := sherpa.NewSpeakerEmbeddingExtractor(&sherpa.SpeakerEmbeddingExtractorConfig{Model: model, NumThreads: threads, Provider: "cpu"})
		if extractor == nil {
			t.Fatal("voiceprint extractor")
		}
		for range 3 {
			if err := perfEmbed(extractor, window); err != nil {
				t.Fatal(err)
			}
		}
		samples := make([]float64, 20)
		for i := range samples {
			started := time.Now()
			if err := perfEmbed(extractor, window); err != nil {
				t.Fatal(err)
			}
			samples[i] = float64(time.Since(started).Microseconds()) / 1000
		}
		sherpa.DeleteSpeakerEmbeddingExtractor(extractor)
		mean, p50, p95 := perfSummary(samples)
		rows = append(rows, perfThread{Threads: threads, WindowMs: 1500, Warmup: 3, Repeats: len(samples), MeanMs: mean, P50Ms: p50, P95Ms: p95})
	}
	return rows
}

func perfEmbed(extractor *sherpa.SpeakerEmbeddingExtractor, samples []float32) error {
	stream := extractor.CreateStream()
	if stream == nil {
		return errPerfEmbed
	}
	defer sherpa.DeleteOnlineStream(stream)
	stream.AcceptWaveform(voiceSampleRate, samples)
	stream.InputFinished()
	if !extractor.IsReady(stream) || !validVectors([][]float32{extractor.Compute(stream)}) {
		return errPerfEmbed
	}
	return nil
}

var errPerfEmbed = errString("embedding failed")

type errString string

func (e errString) Error() string { return string(e) }
