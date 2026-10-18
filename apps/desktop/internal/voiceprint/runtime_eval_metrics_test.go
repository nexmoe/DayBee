package voiceprint

import (
	"fmt"
	"math"
	"testing"
)

// runtimeEvalDur is a sample count inside one labeled region.
type runtimeEvalDur struct {
	Samples, Me, NotMe, Uncertain int
}

func runtimeEvalCoverage(spans []IdentitySpan, n int) error {
	if n <= 0 {
		return fmt.Errorf("empty audio")
	}
	end := 0
	for _, span := range spans {
		if span.Start != end || span.End <= span.Start || span.End > n {
			return fmt.Errorf("coverage %d:%d after %d/%d", span.Start, span.End, end, n)
		}
		switch span.Identity {
		case spanMe, spanNotMe, spanUncertain:
		default:
			return fmt.Errorf("identity %q", span.Identity)
		}
		if !finite32(span.Score) {
			return fmt.Errorf("non-finite score")
		}
		end = span.End
	}
	if end != n {
		return fmt.Errorf("truncated at %d of %d", end, n)
	}
	return nil
}

func runtimeEvalCount(spans []IdentitySpan, start, end int) runtimeEvalDur {
	var dur runtimeEvalDur
	if end < start {
		return dur
	}
	dur.Samples = end - start
	for _, span := range spans {
		lo, hi := span.Start, span.End
		if lo < start {
			lo = start
		}
		if hi > end {
			hi = end
		}
		if hi <= lo {
			continue
		}
		switch span.Identity {
		case spanMe:
			dur.Me += hi - lo
		case spanNotMe:
			dur.NotMe += hi - lo
		default:
			dur.Uncertain += hi - lo
		}
	}
	return dur
}

// runtimeEvalTotals weights every rate by sample duration.
// Mixed speech has its own numerator and is not part of the impostor FAR.
// FRR counts not_me and uncertain on speech whose truth is me.
type runtimeEvalTotals struct {
	Samples, Me, NotMe, Uncertain int
	MeSamples, MeHit, MeMiss      int
	FARSamples, FARMe             int
	MixedSamples, MixedMe         int
	Trials, Segments              int
	Milliseconds                  float64
	Boundaries, Missing           int
	Early, Late                   int
	Bracket, Undetected           int
	MatchedAbs                    int
	Matched                       int
}

func (s *runtimeEvalTotals) add(dur runtimeEvalDur, truth string) {
	s.Samples += dur.Samples
	s.Me += dur.Me
	s.NotMe += dur.NotMe
	s.Uncertain += dur.Uncertain
	switch truth {
	case "me":
		s.MeSamples += dur.Samples
		s.MeHit += dur.Me
		s.MeMiss += dur.NotMe + dur.Uncertain
	case "not_me":
		s.FARSamples += dur.Samples
		s.FARMe += dur.Me
	case "mixed":
		s.MixedSamples += dur.Samples
		s.MixedMe += dur.Me
	}
}

func runtimeEvalRatio(num, den int) (float64, bool) {
	if den <= 0 {
		return 0, false
	}
	value := float64(num) / float64(den)
	return value, !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (s runtimeEvalTotals) recall() (float64, bool) {
	return runtimeEvalRatio(s.MeHit, s.MeSamples)
}

func (s runtimeEvalTotals) far() (float64, bool) {
	return runtimeEvalRatio(s.FARMe, s.FARSamples)
}

func (s runtimeEvalTotals) frr() (float64, bool) {
	return runtimeEvalRatio(s.MeMiss, s.MeSamples)
}

func (s runtimeEvalTotals) uncertainRatio() (float64, bool) {
	return runtimeEvalRatio(s.Uncertain, s.Samples)
}

func (s runtimeEvalTotals) mixedRecall() (float64, bool) {
	return runtimeEvalRatio(s.MixedMe, s.MixedSamples)
}

func (s runtimeEvalTotals) matchedMeanSamples() (float64, bool) {
	return runtimeEvalRatio(s.MatchedAbs, s.Matched)
}

// runtimeEvalBracket is the uncertain interval between two identities.
// It is not a point boundary and must not be reported as a zero offset.
type runtimeEvalBracket struct {
	Start       int  `json:"start"`
	End         int  `json:"end"`
	Width       int  `json:"width"`
	TruthInside bool `json:"truthInside"`
}

// runtimeEvalBoundary is one truth change. A missing edge keeps Offset nil
// so an absent boundary cannot be reported as a zero error. An uncertain
// bracket also keeps Missing set: there is still no precise edge.
type runtimeEvalBoundary struct {
	At      int                 `json:"at"`
	Left    string              `json:"left"`
	Right   string              `json:"right"`
	Missing bool                `json:"missing"`
	Edge    *int                `json:"edge,omitempty"`
	Offset  *int                `json:"offset,omitempty"`
	Bracket *runtimeEvalBracket `json:"uncertainBracket,omitempty"`
}

func runtimeEvalBoundaries(spans []IdentitySpan, expected []runtimeEvalBoundary) []runtimeEvalBoundary {
	out := make([]runtimeEvalBoundary, len(expected))
	for i, want := range expected {
		out[i] = runtimeEvalBoundary{At: want.At, Left: want.Left, Right: want.Right, Missing: true}
		best, found := 0, false
		for n := 0; n+1 < len(spans); n++ {
			if spans[n].End != spans[n+1].Start || spans[n].Identity != want.Left || spans[n+1].Identity != want.Right {
				continue
			}
			delta := spans[n].End - want.At
			if delta < 0 {
				delta = -delta
			}
			bestDelta := best - want.At
			if bestDelta < 0 {
				bestDelta = -bestDelta
			}
			if !found || delta < bestDelta {
				best, found = spans[n].End, true
			}
		}
		if !found {
			if bracket, ok := runtimeEvalUncertainBracket(spans, want); ok {
				out[i].Bracket = &bracket
			}
			continue
		}
		edge, offset := best, best-want.At
		out[i].Missing = false
		out[i].Edge = &edge
		out[i].Offset = &offset
	}
	return out
}

func runtimeEvalUncertainBracket(spans []IdentitySpan, want runtimeEvalBoundary) (runtimeEvalBracket, bool) {
	best := runtimeEvalBracket{}
	found := false
	bestDist := 0
	for n := 0; n < len(spans); n++ {
		if spans[n].Identity != want.Left {
			continue
		}
		k := n + 1
		for k < len(spans) && spans[k].Identity == spanUncertain && spans[k].Start == spans[k-1].End {
			k++
		}
		if k >= len(spans) || k == n+1 || spans[k].Identity != want.Right || spans[k].Start != spans[k-1].End {
			continue
		}
		start, end := spans[n].End, spans[k].Start
		if end <= start {
			continue
		}
		inside := want.At >= start && want.At < end
		dist := 0
		if want.At < start {
			dist = start - want.At
		} else if want.At >= end {
			dist = want.At - (end - 1)
		}
		width := end - start
		better := !found || (inside && !best.TruthInside) || (inside == best.TruthInside && (dist < bestDist || (dist == bestDist && width < best.Width)))
		if !better {
			continue
		}
		found = true
		bestDist = dist
		best = runtimeEvalBracket{Start: start, End: end, Width: width, TruthInside: inside}
	}
	return best, found
}

func (s *runtimeEvalTotals) addBoundaries(rows []runtimeEvalBoundary) {
	for _, row := range rows {
		s.Boundaries++
		if row.Missing || row.Offset == nil {
			s.Missing++
			if row.Bracket != nil && row.Bracket.Width > 0 {
				s.Bracket++
			} else {
				s.Undetected++
			}
			continue
		}
		delta := *row.Offset
		if delta < 0 {
			s.Early++
			delta = -delta
		} else if delta > 0 {
			s.Late++
		}
		s.Matched++
		s.MatchedAbs += delta
	}
}

func TestRuntimeEvalMetricDenominators(t *testing.T) {
	me := []IdentitySpan{
		{Start: 0, End: 100, Identity: spanMe, Score: 1},
		{Start: 100, End: 150, Identity: spanUncertain, Score: 0},
		{Start: 150, End: 200, Identity: spanNotMe, Score: -1},
	}
	if err := runtimeEvalCoverage(me, 200); err != nil {
		t.Fatal(err)
	}
	other := []IdentitySpan{
		{Start: 0, End: 40, Identity: spanMe, Score: 1},
		{Start: 40, End: 200, Identity: spanNotMe, Score: 0},
	}
	mixed := []IdentitySpan{
		{Start: 0, End: 80, Identity: spanMe, Score: 1},
		{Start: 80, End: 200, Identity: spanUncertain, Score: 0},
	}
	var totals runtimeEvalTotals
	totals.add(runtimeEvalCount(me, 0, 200), "me")
	totals.add(runtimeEvalCount(other, 0, 200), "not_me")
	totals.add(runtimeEvalCount(mixed, 0, 200), "mixed")
	recall, ok := totals.recall()
	far, farOK := totals.far()
	frr, frrOK := totals.frr()
	uncertain, uncertainOK := totals.uncertainRatio()
	mixedRecall, mixedOK := totals.mixedRecall()
	if !ok || !farOK || !frrOK || !uncertainOK || !mixedOK {
		t.Fatal("missing rate")
	}
	if recall != 0.5 || frr != 0.5 || far != 0.2 || mixedRecall != 0.4 {
		t.Fatalf("recall %.3f frr %.3f far %.3f mixed %.3f", recall, frr, far, mixedRecall)
	}
	if totals.FARSamples != 200 || totals.MeSamples != 200 || totals.MixedSamples != 200 {
		t.Fatalf("denominators mixed into FAR: %+v", totals)
	}
	// Uncertain on the me and mixed clips is 50+120; the impostor clip adds 0.
	if uncertain != 170.0/600.0 {
		t.Fatalf("uncertain %.4f", uncertain)
	}
	overlap := []IdentitySpan{{Start: 0, End: 10, Identity: spanMe, Score: 1}, {Start: 9, End: 20, Identity: spanNotMe, Score: 0}}
	if runtimeEvalCoverage(overlap, 20) == nil {
		t.Fatal("overlap was accepted")
	}
	gap := []IdentitySpan{{Start: 0, End: 10, Identity: spanMe, Score: 1}, {Start: 12, End: 20, Identity: spanNotMe, Score: 0}}
	if runtimeEvalCoverage(gap, 20) == nil {
		t.Fatal("gap was accepted")
	}
	nan := []IdentitySpan{{Start: 0, End: 4, Identity: spanMe, Score: float32(math.NaN())}}
	if runtimeEvalCoverage(nan, 4) == nil {
		t.Fatal("non-finite score was accepted")
	}
}

func TestRuntimeEvalMissingBoundaryIsNotZero(t *testing.T) {
	perfect := []IdentitySpan{
		{Start: 0, End: 64000, Identity: spanMe, Score: 1},
		{Start: 64000, End: 128000, Identity: spanNotMe, Score: 0},
		{Start: 128000, End: 192000, Identity: spanMe, Score: 1},
	}
	expect := []runtimeEvalBoundary{
		{At: 64000, Left: spanMe, Right: spanNotMe},
		{At: 128000, Left: spanNotMe, Right: spanMe},
	}
	got := runtimeEvalBoundaries(perfect, expect)
	if got[0].Missing || got[1].Missing || *got[0].Offset != 0 || *got[1].Offset != 0 {
		t.Fatalf("%+v", got)
	}
	shifted := []IdentitySpan{
		{Start: 0, End: 70000, Identity: spanMe, Score: 1},
		{Start: 70000, End: 120000, Identity: spanNotMe, Score: 0},
		{Start: 120000, End: 192000, Identity: spanMe, Score: 1},
	}
	got = runtimeEvalBoundaries(shifted, expect)
	if got[0].Missing || got[1].Missing || *got[0].Offset != 6000 || *got[1].Offset != -8000 {
		t.Fatalf("%+v %d %d", got, *got[0].Offset, *got[1].Offset)
	}
	var totals runtimeEvalTotals
	totals.addBoundaries(got)
	mean, ok := totals.matchedMeanSamples()
	if !ok || totals.Missing != 0 || mean != 7000 {
		t.Fatalf("mean %.1f missing %d", mean, totals.Missing)
	}
	whole := []IdentitySpan{{Start: 0, End: 192000, Identity: spanMe, Score: 1}}
	got = runtimeEvalBoundaries(whole, expect)
	if !got[0].Missing || !got[1].Missing || got[0].Offset != nil || got[1].Offset != nil {
		t.Fatalf("missing boundary collapsed to an offset: %+v", got)
	}
	totals = runtimeEvalTotals{}
	totals.addBoundaries(got)
	miss, missOK := runtimeEvalRatio(totals.Missing, totals.Boundaries)
	if _, ok := totals.matchedMeanSamples(); ok || !missOK || miss != 1 || totals.Missing != 2 || totals.Boundaries != 2 {
		t.Fatalf("missing boundaries were dropped from the denominator: %+v", totals)
	}
	totals = runtimeEvalTotals{}
	totals.addBoundaries(runtimeEvalBoundaries(shifted, expect))
	if totals.Boundaries != 2 || totals.Missing != 0 || totals.Early != 1 || totals.Late != 1 {
		t.Fatalf("early boundary left the denominator: %+v", totals)
	}
}

type runtimeEvalReport struct {
	Limitation       string                `json:"limitation"`
	Audio            string                `json:"audio"`
	VAD              bool                  `json:"vad"`
	Model            string                `json:"model"`
	WAVDir           string                `json:"wavDir"`
	Device           string                `json:"device"`
	EchoCancellation bool                  `json:"echoCancellation"`
	Seed             uint64                `json:"seed"`
	NoiseDB          float64               `json:"noiseDb"`
	ReverbDelayMs    int                   `json:"reverbDelayMs"`
	WetGain          float32               `json:"wetGain"`
	RejectionRate    float64               `json:"rejectionRate"`
	Speakers         []runtimeEvalRow      `json:"speakers"`
	Trials           []runtimeEvalTrial    `json:"trials"`
	Aggregates       []runtimeEvalKindStat `json:"aggregates"`
	Benefit          []runtimeEvalBenefit  `json:"benefit"`
}

type runtimeEvalRow struct {
	ID              string     `json:"id"`
	Split           string     `json:"split"`
	Audio           string     `json:"audio"`
	TestFiles       []string   `json:"testFiles"`
	EnrollmentFiles [][]string `json:"enrollmentFiles,omitempty"`
	TestSeconds     float64    `json:"testSeconds"`
	GroupSeconds    []float64  `json:"groupSeconds,omitempty"`
	SkippedShort    int        `json:"skippedShort,omitempty"`
	Enrolled        bool       `json:"enrolled"`
	Error           string     `json:"error,omitempty"`
	TLow            float32    `json:"tLow,omitempty"`
	THigh           float32    `json:"tHigh,omitempty"`
	GenuineMin      float64    `json:"genuineMin,omitempty"`
	GenuineMax      float64    `json:"genuineMax,omitempty"`
	ImpostorMax     float64    `json:"impostorMax,omitempty"`
}

// runtimeEvalRegionDur keeps the truth interval in Start and End.
// Both ends are serialized; a miss does not remove the interval.
type runtimeEvalRegionDur struct {
	Start     int      `json:"start"`
	End       int      `json:"end"`
	Truth     string   `json:"truth"`
	Me        int      `json:"me"`
	NotMe     int      `json:"notMe"`
	Uncertain int      `json:"uncertain"`
	Recall    *float64 `json:"recall,omitempty"`
	FAR       *float64 `json:"far,omitempty"`
	FRR       *float64 `json:"frr,omitempty"`
}

type runtimeEvalTrial struct {
	Speaker         string                 `json:"speaker"`
	Adaptation      bool                   `json:"adaptation"`
	Kind            string                 `json:"kind"`
	Bucket          string                 `json:"bucket,omitempty"`
	Other           string                 `json:"other,omitempty"`
	Truth           string                 `json:"truth"`
	Note            string                 `json:"note,omitempty"`
	Samples         int                    `json:"samples"`
	Me              int                    `json:"me"`
	NotMe           int                    `json:"notMe"`
	Uncertain       int                    `json:"uncertain"`
	MeFraction      float64                `json:"meFraction"`
	Recall          *float64               `json:"recall,omitempty"`
	FAR             *float64               `json:"far,omitempty"`
	FRR             *float64               `json:"frr,omitempty"`
	MixedRecall     *float64               `json:"mixedRecall,omitempty"`
	UncertainRatio  float64                `json:"uncertainRatio"`
	Segments        int                    `json:"segments"`
	Milliseconds    float64                `json:"milliseconds"`
	AdaptationCount int                    `json:"adaptationCount"`
	SystemCount     int                    `json:"systemCount"`
	Counted         bool                   `json:"counted"`
	FunctionalError string                 `json:"functionalError,omitempty"`
	ExpectedEdges   int                    `json:"expectedEdges"`
	MissingEdges    int                    `json:"missingEdges"`
	EarlyEdges      int                    `json:"earlyEdges"`
	LateEdges       int                    `json:"lateEdges"`
	BracketEdges    int                    `json:"uncertainBrackets"`
	UndetectedEdges int                    `json:"undetectedEdges"`
	Boundaries      []runtimeEvalBoundary  `json:"boundaries,omitempty"`
	Spans           []IdentitySpan         `json:"spans"`
	Regions         []runtimeEvalRegionDur `json:"regions,omitempty"`
}

type runtimeEvalKindStat struct {
	Adaptation     bool     `json:"adaptation"`
	Kind           string   `json:"kind"`
	Bucket         string   `json:"bucket,omitempty"`
	Trials         int      `json:"trials"`
	Samples        int      `json:"samples"`
	MeFraction     float64  `json:"meFraction"`
	Recall         *float64 `json:"recall,omitempty"`
	FAR            *float64 `json:"far,omitempty"`
	FRR            *float64 `json:"frr,omitempty"`
	MixedRecall    *float64 `json:"mixedRecall,omitempty"`
	UncertainRatio float64  `json:"uncertainRatio"`
	Segments       int      `json:"segments"`
	Milliseconds   float64  `json:"milliseconds"`
	Boundaries     int      `json:"boundaries"`
	MissingEdges   int      `json:"missingEdges"`
	EarlyEdges     int      `json:"earlyEdges"`
	LateEdges      int      `json:"lateEdges"`
	BracketEdges   int      `json:"uncertainBrackets"`
	Undetected     int      `json:"undetectedEdges"`
	MissRate       *float64 `json:"boundaryMissRate,omitempty"`
	MatchedMeanSec *float64 `json:"matchedOnlyMeanAbsSeconds,omitempty"`
}

type runtimeEvalBenefit struct {
	Kind           string   `json:"kind"`
	Bucket         string   `json:"bucket,omitempty"`
	RecallDelta    *float64 `json:"recallDelta,omitempty"`
	FARDelta       *float64 `json:"farDelta,omitempty"`
	FRRDelta       *float64 `json:"frrDelta,omitempty"`
	AdaptationMax  int      `json:"adaptationMax"`
	SystemMax      int      `json:"systemMax"`
	Interpretation string   `json:"interpretation"`
}
