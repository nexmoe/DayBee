package voiceprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const runtimeEvalLimitation = "四人只对本批 cohort 和个人注册留出。生产模型和 0.537 原始阈值的历史横评见过这套约 13 人语料，所以这里是生产判定回归，不是未见说话人上的无偏准确率。注册和测试都是整句切出的 raw PCM，没有用 VAD 取净语音。噪声和混响是固定种子的合成，不能代表真实房间或本机麦克风。"

// TestNativeRuntimeEvaluation probes production MatchSpansContext.
// It skips unless DAYBEE_VOICEPRINT_PIPELINE_MODEL and DAYBEE_VOICEPRINT_PIPELINE_WAV are set.
// DAYBEE_VOICEPRINT_RUNTIME_OUT writes the report. Suggested path:
// /tmp/voiceprint-runtime-eval-20261012.json. Thresholds stay unchanged.
func TestNativeRuntimeEvaluation(t *testing.T) {
	model, dir := os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_MODEL"), os.Getenv("DAYBEE_VOICEPRINT_PIPELINE_WAV")
	if model == "" || dir == "" {
		t.Skip("Set DAYBEE_VOICEPRINT_PIPELINE_MODEL and DAYBEE_VOICEPRINT_PIPELINE_WAV")
	}
	speakers, err := runtimeEvalLoad(dir)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := New(t.TempDir(), func() string { return model })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	enrolled := 0
	for i := range speakers {
		runtimeEvalEnroll(t, manager, &speakers[i])
		if speakers[i].Profile != nil {
			enrolled++
		}
		t.Logf("%s enrolled=%v test=%.2fs %s", speakers[i].ID, speakers[i].Profile != nil, float64(len(speakers[i].Test))/voiceSampleRate, speakers[i].Error)
	}
	report := runtimeEvalReport{
		Limitation: runtimeEvalLimitation, Audio: "raw", VAD: false, Model: model, WAVDir: dir,
		Device: runtimeEvalContext.Device, EchoCancellation: runtimeEvalContext.EchoCancellation,
		Seed: runtimeEvalSeed, NoiseDB: runtimeEvalSNR, ReverbDelayMs: runtimeEvalDelayMs, WetGain: runtimeEvalWet,
		RejectionRate: float64(len(speakers)-enrolled) / float64(len(speakers)),
		Speakers:      runtimeEvalRows(speakers),
	}
	var trials []runtimeEvalTrial
	for i := range speakers {
		if speakers[i].Profile == nil {
			continue
		}
		if err := manager.SaveProfile(speakers[i].Profile); err != nil {
			t.Errorf("%s save: %v", speakers[i].ID, err)
			continue
		}
		clips := runtimeEvalClips(speakers[i], speakers)
		for _, enabled := range []bool{false, true} {
			trials = append(trials, runtimeEvalRun(t, manager, speakers[i].ID, clips, enabled)...)
		}
	}
	report.Trials = trials
	report.Aggregates = runtimeEvalAggregate(trials)
	report.Benefit = runtimeEvalCompare(report.Aggregates, trials)
	runtimeEvalLog(t, report)
	if out := os.Getenv("DAYBEE_VOICEPRINT_RUNTIME_OUT"); out != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, append(data, '\n'), 0600); err != nil {
			t.Errorf("write report: %v", err)
		}
	}
}

func runtimeEvalLoad(dir string) ([]runtimeEvalSpeaker, error) {
	ids := []string{"SSB0544", "SSB0700", "SSB0693", "SSB0434"}
	speakers := make([]runtimeEvalSpeaker, len(ids))
	for i, id := range ids {
		paths, err := filepath.Glob(filepath.Join(dir, id+"_*.wav"))
		if err != nil {
			return nil, err
		}
		var files []runtimeEvalUtt
		speaker := runtimeEvalSpeaker{ID: id, Audio: "raw", Split: "highest utterance index until 8s"}
		if id == "SSB0700" {
			speaker.Split = "shortest utterances until 8s, then length-balanced groups"
		}
		for _, path := range paths {
			_, index, ok := evalParseUtt(filepath.Base(path))
			if !ok {
				return nil, fmt.Errorf("unexpected wav name %s", filepath.Base(path))
			}
			pcm, err := evalReadPCM(path)
			if err != nil {
				return nil, err
			}
			if pcm == nil {
				speaker.SkippedShort++
				continue
			}
			files = append(files, runtimeEvalUtt{Name: filepath.Base(path), Index: index, PCM: pcm})
		}
		test, rest, _, holdOK := runtimeEvalHoldout(id, files)
		speaker.Test = runtimeEvalConcat(test)
		for _, file := range test {
			speaker.TestFiles = append(speaker.TestFiles, file.Name)
		}
		if !holdOK {
			speaker.Error = "held-out raw speech is under 8s"
			speakers[i] = speaker
			continue
		}
		groups, lengths, groupsOK := runtimeEvalGroups(rest)
		speaker.EnrollmentFiles = make([][]string, 3)
		speaker.GroupSeconds = make([]float64, 3)
		for group, n := range lengths {
			speaker.GroupSeconds[group] = float64(n) / voiceSampleRate
			for _, file := range groups[group] {
				speaker.EnrollmentFiles[group] = append(speaker.EnrollmentFiles[group], file.Name)
			}
		}
		if !groupsOK {
			speaker.Error = "three raw 8s enrollment groups were not available"
			speakers[i] = speaker
			continue
		}
		need := runtimeEvalGroupSec * voiceSampleRate
		for group, files := range groups {
			speaker.groups[group] = runtimeEvalConcat(files)[:need]
		}
		speakers[i] = speaker
	}
	return speakers, nil
}

func runtimeEvalEnroll(t *testing.T, m *Manager, speaker *runtimeEvalSpeaker) {
	if speaker.Error != "" {
		return
	}
	groups := make([][][]float32, 3)
	for i, pcm := range speaker.groups {
		vectors, _, err := m.EmbedWindows(pcm, voiceSampleRate)
		if err != nil {
			speaker.Error = err.Error()
			t.Errorf("%s embed: %v", speaker.ID, err)
			return
		}
		groups[i] = vectors
	}
	profile, err := Calibrate(groups, runtimeEvalContext)
	if err == nil {
		speaker.Profile = profile
		return
	}
	speaker.Error = err.Error()
	if errors.Is(err, ErrEnrollmentInseparable) {
		runtimeEvalNoteSeparation(speaker, groups)
		return
	}
	t.Errorf("%s calibrate: %v", speaker.ID, err)
}

func runtimeEvalNoteSeparation(speaker *runtimeEvalSpeaker, groups [][][]float32) {
	cohort, meta, err := LoadCohort()
	if err != nil {
		return
	}
	genuine, impostor, err := crossScores(groups, cohort, meta.Windows)
	if err != nil || len(genuine) == 0 || len(impostor) == 0 {
		return
	}
	sort.Float64s(genuine)
	sort.Float64s(impostor)
	speaker.GenuineMin = genuine[0]
	speaker.GenuineMax = genuine[len(genuine)-1]
	speaker.ImpostorMax = impostor[len(impostor)-1]
}

func runtimeEvalRun(t *testing.T, m *Manager, speaker string, clips []runtimeEvalClip, enabled bool) []runtimeEvalTrial {
	m.SetAdaptationEnabled(enabled)
	if err := m.ClearAdaptation(); err != nil {
		t.Errorf("%s clear adaptation: %v", speaker, err)
	}
	m.ResetRuntime(runtimeEvalContext)
	trials := make([]runtimeEvalTrial, 0, len(clips))
	for _, clip := range clips {
		started := time.Now()
		spans := m.MatchSpansContext(m.Snapshot(), clip.Samples, voiceSampleRate, clip.Ref, clip.Context, false)
		elapsed := float64(time.Since(started).Microseconds()) / 1000
		adaptN, systemN := m.RuntimeCounts()
		trial := runtimeEvalMeasure(speaker, enabled, clip, spans, elapsed, adaptN, systemN)
		if trial.FunctionalError != "" {
			t.Errorf("%s adapt=%v %s %s: %s", speaker, enabled, clip.Kind, clip.Bucket, trial.FunctionalError)
		}
		trials = append(trials, trial)
	}
	return trials
}

func runtimeEvalMeasure(speaker string, enabled bool, clip runtimeEvalClip, spans []IdentitySpan, elapsed float64, adaptN, systemN int) runtimeEvalTrial {
	trial := runtimeEvalTrial{
		Speaker: speaker, Adaptation: enabled, Kind: clip.Kind, Bucket: clip.Bucket, Other: clip.Other,
		Truth: clip.Truth, Note: clip.Note, Segments: len(spans), Milliseconds: elapsed,
		AdaptationCount: adaptN, SystemCount: systemN, Spans: spans, Counted: true,
		Samples: len(clip.Samples),
	}
	if err := runtimeEvalCoverage(spans, len(clip.Samples)); err != nil {
		trial.FunctionalError = err.Error()
		trial.Counted = false
		return trial
	}
	whole := runtimeEvalCount(spans, 0, len(clip.Samples))
	trial.Samples, trial.Me, trial.NotMe, trial.Uncertain = whole.Samples, whole.Me, whole.NotMe, whole.Uncertain
	trial.MeFraction = float64(whole.Me) / float64(whole.Samples)
	trial.UncertainRatio = float64(whole.Uncertain) / float64(whole.Samples)
	if clip.ExpectNoMe && whole.Me != 0 {
		trial.FunctionalError = "mismatched context returned me"
	}
	switch clip.Truth {
	case "me":
		trial.Recall = runtimeEvalMust(whole.Me, whole.Samples)
		trial.FRR = runtimeEvalMust(whole.NotMe+whole.Uncertain, whole.Samples)
	case "not_me":
		trial.FAR = runtimeEvalMust(whole.Me, whole.Samples)
	case "mixed":
		trial.MixedRecall = runtimeEvalMust(whole.Me, whole.Samples)
	case "sequence":
		var totals runtimeEvalTotals
		for _, region := range clip.Regions {
			dur := runtimeEvalCount(spans, region.Start, region.End)
			totals.add(dur, region.Truth)
			trial.Regions = append(trial.Regions, runtimeEvalRegionDur{
				Start: region.Start, End: region.End, Truth: region.Truth,
				Me: dur.Me, NotMe: dur.NotMe, Uncertain: dur.Uncertain,
				Recall: runtimeEvalRate("me", region.Truth, dur), FAR: runtimeEvalRate("not_me", region.Truth, dur),
				FRR: runtimeEvalMiss(region.Truth, dur),
			})
		}
		trial.Recall = runtimeEvalOptional(totals.recall())
		trial.FAR = runtimeEvalOptional(totals.far())
		trial.FRR = runtimeEvalOptional(totals.frr())
		trial.Boundaries = runtimeEvalBoundaries(spans, clip.Expected)
		var edges runtimeEvalTotals
		edges.addBoundaries(trial.Boundaries)
		trial.ExpectedEdges = edges.Boundaries
		trial.MissingEdges = edges.Missing
		trial.EarlyEdges = edges.Early
		trial.LateEdges = edges.Late
		trial.BracketEdges = edges.Bracket
		trial.UndetectedEdges = edges.Undetected
	}
	for _, value := range []*float64{trial.Recall, trial.FAR, trial.FRR, trial.MixedRecall} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			trial.FunctionalError = "non-finite rate"
			trial.Counted = false
		}
	}
	return trial
}

func runtimeEvalMust(num, den int) *float64 {
	value, ok := runtimeEvalRatio(num, den)
	if !ok {
		return nil
	}
	return &value
}

func runtimeEvalOptional(value float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &value
}

func runtimeEvalRate(want, truth string, dur runtimeEvalDur) *float64 {
	if truth != want {
		return nil
	}
	if want == "me" {
		return runtimeEvalMust(dur.Me, dur.Samples)
	}
	return runtimeEvalMust(dur.Me, dur.Samples)
}

func runtimeEvalMiss(truth string, dur runtimeEvalDur) *float64 {
	if truth != "me" {
		return nil
	}
	return runtimeEvalMust(dur.NotMe+dur.Uncertain, dur.Samples)
}

func runtimeEvalRows(speakers []runtimeEvalSpeaker) []runtimeEvalRow {
	rows := make([]runtimeEvalRow, len(speakers))
	for i, speaker := range speakers {
		row := runtimeEvalRow{
			ID: speaker.ID, Split: speaker.Split, Audio: speaker.Audio, TestFiles: speaker.TestFiles,
			EnrollmentFiles: speaker.EnrollmentFiles, TestSeconds: float64(len(speaker.Test)) / voiceSampleRate,
			GroupSeconds: speaker.GroupSeconds, SkippedShort: speaker.SkippedShort, Enrolled: speaker.Profile != nil,
			Error: speaker.Error, GenuineMin: speaker.GenuineMin, GenuineMax: speaker.GenuineMax, ImpostorMax: speaker.ImpostorMax,
		}
		if speaker.Profile != nil && speaker.Profile.Calibration != nil {
			row.TLow = speaker.Profile.Calibration.TLow
			row.THigh = speaker.Profile.Calibration.THigh
		}
		rows[i] = row
	}
	return rows
}

func runtimeEvalAggregate(trials []runtimeEvalTrial) []runtimeEvalKindStat {
	order := make([]string, 0)
	by := map[string]*struct {
		stat   runtimeEvalKindStat
		totals runtimeEvalTotals
	}{}
	for _, trial := range trials {
		if !trial.Counted {
			continue
		}
		key := fmt.Sprintf("%v\x00%s\x00%s", trial.Adaptation, trial.Kind, trial.Bucket)
		row := by[key]
		if row == nil {
			row = &struct {
				stat   runtimeEvalKindStat
				totals runtimeEvalTotals
			}{stat: runtimeEvalKindStat{Adaptation: trial.Adaptation, Kind: trial.Kind, Bucket: trial.Bucket}}
			by[key] = row
			order = append(order, key)
		}
		row.stat.Trials++
		row.stat.Segments += trial.Segments
		row.stat.Milliseconds += trial.Milliseconds
		if trial.Truth == "sequence" {
			for _, region := range trial.Regions {
				row.totals.add(runtimeEvalDur{Samples: region.End - region.Start, Me: region.Me, NotMe: region.NotMe, Uncertain: region.Uncertain}, region.Truth)
			}
			row.totals.addBoundaries(trial.Boundaries)
			continue
		}
		row.totals.add(runtimeEvalDur{Samples: trial.Samples, Me: trial.Me, NotMe: trial.NotMe, Uncertain: trial.Uncertain}, trial.Truth)
	}
	out := make([]runtimeEvalKindStat, 0, len(order))
	for _, key := range order {
		row := by[key]
		stat := row.stat
		stat.Samples = row.totals.Samples
		if ratio, ok := runtimeEvalRatio(row.totals.Me, row.totals.Samples); ok {
			stat.MeFraction = ratio
		}
		if ratio, ok := row.totals.uncertainRatio(); ok {
			stat.UncertainRatio = ratio
		}
		stat.Recall = runtimeEvalOptional(row.totals.recall())
		stat.FAR = runtimeEvalOptional(row.totals.far())
		stat.FRR = runtimeEvalOptional(row.totals.frr())
		stat.MixedRecall = runtimeEvalOptional(row.totals.mixedRecall())
		stat.Boundaries = row.totals.Boundaries
		stat.MissingEdges = row.totals.Missing
		stat.EarlyEdges = row.totals.Early
		stat.LateEdges = row.totals.Late
		stat.BracketEdges = row.totals.Bracket
		stat.Undetected = row.totals.Undetected
		stat.MissRate = runtimeEvalOptional(runtimeEvalRatio(row.totals.Missing, row.totals.Boundaries))
		if mean, ok := row.totals.matchedMeanSamples(); ok {
			seconds := mean / voiceSampleRate
			stat.MatchedMeanSec = &seconds
		}
		out = append(out, stat)
	}
	return out
}

func runtimeEvalCompare(stats []runtimeEvalKindStat, trials []runtimeEvalTrial) []runtimeEvalBenefit {
	off := map[string]runtimeEvalKindStat{}
	on := map[string]runtimeEvalKindStat{}
	var keys []string
	for _, stat := range stats {
		key := stat.Kind + "\x00" + stat.Bucket
		if stat.Adaptation {
			if _, seen := on[key]; !seen {
				keys = append(keys, key)
			}
			on[key] = stat
		} else {
			off[key] = stat
		}
	}
	out := make([]runtimeEvalBenefit, 0, len(keys))
	for _, key := range keys {
		benefit := runtimeEvalBenefit{Kind: on[key].Kind, Bucket: on[key].Bucket}
		benefit.RecallDelta = runtimeEvalDelta(on[key].Recall, off[key].Recall)
		benefit.FARDelta = runtimeEvalDelta(on[key].FAR, off[key].FAR)
		benefit.FRRDelta = runtimeEvalDelta(on[key].FRR, off[key].FRR)
		for _, trial := range trials {
			if !trial.Adaptation || trial.Kind != benefit.Kind || trial.Bucket != benefit.Bucket {
				continue
			}
			if trial.AdaptationCount > benefit.AdaptationMax {
				benefit.AdaptationMax = trial.AdaptationCount
			}
			if trial.SystemCount > benefit.SystemMax {
				benefit.SystemMax = trial.SystemCount
			}
		}
		benefit.Interpretation = runtimeEvalInterpret(benefit)
		out = append(out, benefit)
	}
	return out
}

func runtimeEvalDelta(on, off *float64) *float64 {
	if on == nil || off == nil {
		return nil
	}
	value := *on - *off
	return &value
}

func runtimeEvalInterpret(benefit runtimeEvalBenefit) string {
	if benefit.AdaptationMax == 0 {
		return "开启后的适应池实际数量仍是 0。off/on 的分数差不能当成适应收益；系统池数量另报。"
	}
	recallUp := benefit.RecallDelta != nil && *benefit.RecallDelta > 0
	farDown := benefit.FARDelta == nil || *benefit.FARDelta <= 0
	if recallUp && farDown {
		return "适应池有向量，本次数值上召回更高且 FAR 没有变差。这不是显著性结论，也不外推到未见说话人。"
	}
	return "适应池有向量，但召回或 FAR 没有同时变好。阈值保持原值。"
}

func runtimeEvalLog(t *testing.T, report runtimeEvalReport) {
	t.Helper()
	t.Logf("limitation: %s", report.Limitation)
	t.Logf("rejectionRate %.3f audio=%s vad=%v seed=%d noise=%.0fdB reverb=%dms wet=%.2f", report.RejectionRate, report.Audio, report.VAD, report.Seed, report.NoiseDB, report.ReverbDelayMs, report.WetGain)
	for _, row := range report.Speakers {
		t.Logf("speaker %s enrolled=%v test=%.2fs split=%s %s", row.ID, row.Enrolled, row.TestSeconds, row.Split, row.Error)
	}
	for _, stat := range report.Aggregates {
		t.Logf("adapt=%v %s %s samples=%d recall=%s far=%s frr=%s mixed=%s uncertain=%.3f segments=%d %.1fms edges=%d missing=%d early=%d late=%d bracket=%d undetected=%d missRate=%s matchedOnly=%s",
			stat.Adaptation, stat.Kind, stat.Bucket, stat.Samples, runtimeEvalText(stat.Recall), runtimeEvalText(stat.FAR), runtimeEvalText(stat.FRR), runtimeEvalText(stat.MixedRecall), stat.UncertainRatio, stat.Segments, stat.Milliseconds, stat.Boundaries, stat.MissingEdges, stat.EarlyEdges, stat.LateEdges, stat.BracketEdges, stat.Undetected, runtimeEvalText(stat.MissRate), runtimeEvalText(stat.MatchedMeanSec))
	}
	for _, benefit := range report.Benefit {
		t.Logf("benefit %s %s adaptMax=%d systemMax=%d recallDelta=%s farDelta=%s %s", benefit.Kind, benefit.Bucket, benefit.AdaptationMax, benefit.SystemMax, runtimeEvalText(benefit.RecallDelta), runtimeEvalText(benefit.FARDelta), benefit.Interpretation)
	}
}

func runtimeEvalText(value *float64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%.4f", *value)
}

func TestRuntimeEvalAggregateKeepsMissedBoundarySamples(t *testing.T) {
	early := -1000
	edge := 7000
	trial := runtimeEvalTrial{
		Counted: true, Adaptation: true, Kind: "sequence", Truth: "sequence", Segments: 1,
		Regions: []runtimeEvalRegionDur{
			{Start: 0, End: 4, Truth: "me", Me: 4},
			{Start: 4, End: 8, Truth: "not_me", NotMe: 4},
			{Start: 8, End: 12, Truth: "me", Uncertain: 4},
		},
		Boundaries: []runtimeEvalBoundary{
			{At: 4, Left: spanMe, Right: spanNotMe, Missing: true},
			{At: 8, Left: spanNotMe, Right: spanMe, Edge: &edge, Offset: &early},
		},
	}
	stats := runtimeEvalAggregate([]runtimeEvalTrial{trial})
	encoded, err := json.Marshal(trial.Regions[0])
	if err != nil || !strings.Contains(string(encoded), `"start":0`) || !strings.Contains(string(encoded), `"end":4`) || strings.Contains(string(encoded), "endSample") {
		t.Fatalf("region interval %s %v", encoded, err)
	}
	if len(stats) != 1 || stats[0].Samples != 12 || stats[0].Boundaries != 2 || stats[0].MissingEdges != 1 || stats[0].EarlyEdges != 1 {
		t.Fatalf("%+v", stats[0])
	}
	if stats[0].Recall == nil || *stats[0].Recall != 0.5 || stats[0].FAR == nil || *stats[0].FAR != 0 || stats[0].MissRate == nil || *stats[0].MissRate != 0.5 {
		t.Fatalf("denominator changed recall=%v far=%v miss=%v", stats[0].Recall, stats[0].FAR, stats[0].MissRate)
	}
	if stats[0].FRR == nil || *stats[0].FRR != 0.5 {
		t.Fatalf("uncertain me region left FRR: %v", stats[0].FRR)
	}
}
