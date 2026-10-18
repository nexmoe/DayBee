package voiceprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// This is an independent production-path acceptance probe, not a threshold sweep.
// Its four speakers are absent from the bundled cohort. Whole utterances belong
// to exactly one enrollment group or to the held-out test audio.
func TestNativeCalibratedPipelineContract(t *testing.T) {
	model, dir := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_MODEL"), os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_WAV")
	if model == "" || dir == "" {
		t.Skip("Set DAYBEE_VOICEPRINT_PIPELINE_MODEL and DAYBEE_VOICEPRINT_PIPELINE_WAV")
	}
	ids := []string{"SSB0544", "SSB0700", "SSB0693", "SSB0434"}
	type speaker struct {
		ID               string
		EnrollmentFiles  [][]string
		TestFiles        []string
		CalibrationError string    `json:"calibrationError,omitempty"`
		Profile          *Profile  `json:"-"`
		Test             []float32 `json:"-"`
	}
	m, err := New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	speakers := make([]speaker, len(ids))
	for i, id := range ids {
		files, err := filepath.Glob(filepath.Join(dir, id+"_*.wav"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(files, func(a, b int) bool {
			_, ai, _ := evalParseUtt(filepath.Base(files[a]))
			_, bi, _ := evalParseUtt(filepath.Base(files[b]))
			return ai < bi
		})
		s := speaker{ID: id}
		pcmByFile := make(map[string][]float32, len(files))
		for _, file := range files {
			pcmByFile[file], err = evalReadPCM(file)
			if err != nil {
				t.Fatal(err)
			}
		}
		// Reserve complete utterances before allocating three balanced takes.
		for len(s.Test) < 4*voiceSampleRate && len(files) > 0 {
			file := files[len(files)-1]
			files = files[:len(files)-1]
			s.Test = append(s.Test, pcmByFile[file]...)
			s.TestFiles = append(s.TestFiles, filepath.Base(file))
		}
		if len(s.Test) < 4*voiceSampleRate {
			t.Fatalf("%s lacks held-out speech", id)
		}
		sort.SliceStable(files, func(a, b int) bool { return len(pcmByFile[files[a]]) > len(pcmByFile[files[b]]) })
		groupPCM := make([][]float32, 3)
		s.EnrollmentFiles = make([][]string, 3)
		for _, file := range files {
			group := 0
			for j := 1; j < 3; j++ {
				if len(groupPCM[j]) < len(groupPCM[group]) {
					group = j
				}
			}
			groupPCM[group] = append(groupPCM[group], pcmByFile[file]...)
			s.EnrollmentFiles[group] = append(s.EnrollmentFiles[group], filepath.Base(file))
		}
		groups := make([][][]float32, 3)
		for group, pcm := range groupPCM {
			if len(pcm) < 8*voiceSampleRate {
				t.Fatalf("%s lacks independent enrollment speech", id)
			}
			groups[group], _, err = m.EmbedWindows(pcm[:8*voiceSampleRate], voiceSampleRate)
			if err != nil {
				t.Fatal(err)
			}
		}
		s.Profile, err = Calibrate(groups, EnrollmentContext{Device: "public-fixture"})
		if err != nil {
			if !errors.Is(err, ErrEnrollmentInseparable) {
				t.Fatal(err)
			}
			s.CalibrationError = err.Error()
			cohort, meta, loadErr := LoadCohort()
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			genuine, impostor, statsErr := crossScores(groups, cohort, meta.Windows)
			if statsErr != nil {
				t.Fatal(statsErr)
			}
			fa, _ := thresholdAtFA(impostor, acceptFalseRate)
			fr, _ := thresholdFRUpper(genuine, rejectFalseRate)
			sort.Float64s(genuine)
			t.Logf("%s registration rejected: FA boundary %.4f > FR boundary %.4f; genuine min=%.4f max=%.4f", id, fa, fr, genuine[0], genuine[len(genuine)-1])
		}
		if s.Profile != nil {
			t.Logf("%s thresholds: %+v", id, s.Profile.Calibration)
		}
		speakers[i] = s
	}
	type trial struct {
		Speaker           string         `json:"speaker"`
		Kind              string         `json:"kind"`
		Seconds           float64        `json:"seconds"`
		MeFraction        float64        `json:"meFraction"`
		UncertainFraction float64        `json:"uncertainFraction"`
		Milliseconds      float64        `json:"milliseconds"`
		Spans             []IdentitySpan `json:"spans"`
	}
	var trials []trial
	for i, s := range speakers {
		if s.Profile == nil {
			continue
		}
		for _, kind := range []string{"clean_me", "other", "own_video"} {
			pcm := s.Test
			var ref *Reference
			if kind == "other" {
				pcm = speakers[(i+1)%len(speakers)].Test
			}
			if kind == "own_video" {
				ref = &Reference{Samples: append([]float32(nil), pcm...)}
				pcm = append([]float32(nil), pcm...)
				for j := range pcm {
					pcm[j] *= .5
				}
			}
			if kind == "other" {
				wins, scoreErr := m.Score(s.Profile, pcm, voiceSampleRate)
				if scoreErr != nil {
					t.Fatal(scoreErr)
				}
				raw := make([]float32, len(wins))
				for j, w := range wins {
					raw[j] = w.Score
				}
				t.Logf("%s other raw=%v normalized=%v", s.ID, raw, DecisionScores(s.Profile, wins))
			}
			started := time.Now()
			spans := m.MatchSpans(s.Profile, pcm, voiceSampleRate, ref)
			row := trial{Speaker: s.ID, Kind: kind, Seconds: float64(len(pcm)) / voiceSampleRate,
				Milliseconds: float64(time.Since(started).Microseconds()) / 1000, Spans: spans}
			end := 0
			for _, span := range spans {
				if span.Start != end || span.End <= span.Start || span.End > len(pcm) || !finite32(span.Score) {
					t.Fatalf("%s/%s invalid coverage: %+v", s.ID, kind, spans)
				}
				fraction := float64(span.End-span.Start) / float64(len(pcm))
				if span.Identity == "me" {
					row.MeFraction += fraction
				}
				if span.Identity == "uncertain" {
					row.UncertainFraction += fraction
				}
				end = span.End
			}
			if end != len(pcm) {
				t.Fatalf("%s/%s truncated coverage", s.ID, kind)
			}
			if (kind == "own_video" || kind == "other") && row.MeFraction != 0 {
				t.Errorf("%s %s marked me", s.ID, kind)
			}
			t.Logf("%s %s me=%.3f uncertain=%.3f %.1fms", s.ID, kind, row.MeFraction, row.UncertainFraction, row.Milliseconds)
			trials = append(trials, row)
		}
	}
	if out := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_OUT"); out != "" {
		report := struct {
			Speakers []speaker `json:"speakers"`
			Trials   []trial   `json:"trials"`
		}{speakers, trials}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
