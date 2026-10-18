package voiceprint

import (
	"math"
	"testing"
)

func TestIdentitySpansMeOtherMe(t *testing.T) {
	const rate = 16000
	low, high := float32(0.3), float32(0.6)

	t.Run("sustained", func(t *testing.T) {
		// 0.5 s hops. Two low windows survive the median and last exactly 1 s.
		windows := []WindowScore{{Start: 0}, {Start: 8000}, {Start: 16000}, {Start: 24000}, {Start: 32000}, {Start: 40000}}
		scores := []float32{0.9, 0.9, 0, 0, 0.9, 0.9}
		spans := IdentitySpans(windows, scores, nil, 48000, rate, low, high)
		assertSpanCover(t, spans, 48000)
		want := []IdentitySpan{
			{Start: 0, End: 16000, Identity: "me", Score: 0.9},
			{Start: 16000, End: 32000, Identity: "not_me", Score: 0},
			{Start: 32000, End: 48000, Identity: "me", Score: 0.9},
		}
		assertSpans(t, spans, want)
	})

	t.Run("under one second stays me", func(t *testing.T) {
		windows := []WindowScore{{Start: 0}, {Start: 16000}, {Start: 32000}, {Start: 36000}, {Start: 40000}}
		scores := []float32{0.9, 0.9, 0, 0, 0.9}
		spans := IdentitySpans(windows, scores, nil, 56000, rate, low, high)
		assertSpanCover(t, spans, 56000)
		if len(spans) != 1 || spans[0].Identity != "me" || spans[0].Start != 0 || spans[0].End != 56000 {
			t.Fatalf("short other was accepted: %+v", spans)
		}
		// 48000 high samples and 8000 smoothed zeros. The dip still changes the score.
		want := float32(float64(float32(0.9)) * 48000 / 56000)
		if math.Abs(float64(spans[0].Score-want)) > 1e-5 {
			t.Fatalf("score = %v, want weighted %v", spans[0].Score, want)
		}
	})

	t.Run("mid band stays me", func(t *testing.T) {
		windows := []WindowScore{{Start: 0}, {Start: 16000}, {Start: 32000}, {Start: 48000}}
		scores := []float32{0.9, 0.4, 0.4, 0.9}
		spans := IdentitySpans(windows, scores, nil, 64000, rate, low, high)
		assertSpanCover(t, spans, 64000)
		if len(spans) != 1 || spans[0].Identity != "me" {
			t.Fatalf("hysteresis left me: %+v", spans)
		}
	})
}

func TestIdentitySpansEchoStaysNotMe(t *testing.T) {
	const rate = 16000
	// Each piece is 0.25 s, so neither side meets the one-second rule alone.
	windows := []WindowScore{{Start: 0}, {Start: 4000}, {Start: 8000}}
	scores := []float32{0.95, 0.95, 0.95}
	echo := []bool{false, true, false}
	spans := IdentitySpans(windows, scores, echo, 12000, rate, 0.3, 0.6)
	assertSpanCover(t, spans, 12000)
	want := []IdentitySpan{
		{Start: 0, End: 4000, Identity: "me", Score: 0.95},
		{Start: 4000, End: 8000, Identity: "not_me", Score: 0.95},
		{Start: 8000, End: 12000, Identity: "me", Score: 0.95},
	}
	assertSpans(t, spans, want)
}

func TestIdentitySpansShortTail(t *testing.T) {
	windows := []WindowScore{{Start: 50, End: 1000}, {Start: 16000, End: 17000}}
	scores := []float32{0.9, 0.05}
	spans := IdentitySpans(windows, scores, nil, 32050, 16000, 0.3, 0.6)
	assertSpanCover(t, spans, 32050)
	want := []IdentitySpan{
		{Start: 0, End: 16000, Identity: "me", Score: 0.9},
		{Start: 16000, End: 32050, Identity: "not_me", Score: 0.05},
	}
	assertSpans(t, spans, want)
	if spans[0].Start == windows[0].Start || spans[1].End == windows[1].End {
		t.Fatalf("span followed window endpoints instead of [0, n): %+v", spans)
	}
}

func TestIdentitySpansUndecided(t *testing.T) {
	if spans := IdentitySpans(nil, nil, nil, 0, 16000, 0.2, 0.6); spans != nil {
		t.Fatalf("empty audio: %#v", spans)
	}
	if spans := IdentitySpans(nil, nil, nil, -1, 16000, 0.2, 0.6); spans != nil {
		t.Fatalf("negative length: %#v", spans)
	}

	windows := []WindowScore{{Start: 0, End: 1000}}
	cases := []struct {
		name    string
		windows []WindowScore
		scores  []float32
		echo    []bool
		n, rate int
		low     float32
		high    float32
	}{
		{name: "no scores", n: 500, rate: 16000, low: 0.2, high: 0.6},
		{name: "bad rate", windows: windows, scores: []float32{0.9}, n: 1000, low: 0.2, high: 0.6},
		{name: "mismatch", windows: windows, scores: []float32{0.9, 0.1}, n: 1000, rate: 16000, low: 0.2, high: 0.6},
		{name: "low above high", windows: windows, scores: []float32{0.9}, n: 1000, rate: 16000, low: 0.8, high: 0.2},
		{name: "start past end", windows: []WindowScore{{Start: 0}, {Start: 10}}, scores: []float32{0.9, 0.1}, n: 5, rate: 16000, low: 0.2, high: 0.6},
		{name: "nan", windows: windows, scores: []float32{float32(math.NaN())}, n: 1000, rate: 16000, low: 0.2, high: 0.6},
		{name: "echo length", windows: windows, scores: []float32{0.9}, echo: []bool{true, false}, n: 1000, rate: 16000, low: 0.2, high: 0.6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans := IdentitySpans(tc.windows, tc.scores, tc.echo, tc.n, tc.rate, tc.low, tc.high)
			assertSpans(t, spans, []IdentitySpan{{Start: 0, End: tc.n, Identity: "uncertain"}})
		})
	}

	t.Run("mid band", func(t *testing.T) {
		windows := []WindowScore{{Start: 0}, {Start: 16000}, {Start: 32000}}
		scores := []float32{0.4, 0.5, 0.45}
		spans := IdentitySpans(windows, scores, nil, 48000, 16000, 0.2, 0.8)
		assertSpanCover(t, spans, 48000)
		if len(spans) != 1 || spans[0].Identity != "uncertain" {
			t.Fatalf("mid band: %+v", spans)
		}
		if math.Abs(float64(spans[0].Score)-(0.4+0.45+0.45)/3) > 1e-4 {
			t.Fatalf("score = %v", spans[0].Score)
		}
	})
}

func TestIdentitySpansContinuousCoverage(t *testing.T) {
	windows := []WindowScore{{Start: 100, End: 50}, {Start: 8000, End: 9000}, {Start: 24000, End: 25000}, {Start: 32000, End: 33000}}
	scores := []float32{0.9, 0, 0, 0.9}
	spans := IdentitySpans(windows, scores, nil, 48010, 16000, 0.3, 0.6)
	assertSpanCover(t, spans, 48010)
	want := []IdentitySpan{
		{Start: 0, End: 8000, Identity: "me", Score: 0.9},
		{Start: 8000, End: 32000, Identity: "not_me", Score: 0},
		{Start: 32000, End: 48010, Identity: "me", Score: 0.9},
	}
	assertSpans(t, spans, want)
	for i := range spans {
		if spans[i].End <= spans[i].Start {
			t.Fatalf("empty span %d: %+v", i, spans[i])
		}
		if i > 0 && spans[i].Start != spans[i-1].End {
			t.Fatalf("break at %d: %+v", i, spans)
		}
	}
}

func assertSpanCover(t *testing.T, spans []IdentitySpan, n int) {
	t.Helper()
	if n == 0 {
		if spans != nil {
			t.Fatalf("empty: %#v", spans)
		}
		return
	}
	if len(spans) == 0 || spans[0].Start != 0 || spans[len(spans)-1].End != n {
		t.Fatalf("coverage of %d: %+v", n, spans)
	}
	for i := range spans {
		if spans[i].End <= spans[i].Start {
			t.Fatalf("empty span: %+v", spans)
		}
		if i > 0 && spans[i].Start != spans[i-1].End {
			t.Fatalf("not contiguous: %+v", spans)
		}
	}
}

func assertSpans(t *testing.T, got, want []IdentitySpan) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i].Start != want[i].Start || got[i].End != want[i].End || got[i].Identity != want[i].Identity {
			t.Fatalf("span %d got %+v want %+v", i, got[i], want[i])
		}
		if math.Abs(float64(got[i].Score-want[i].Score)) > 1e-4 {
			t.Fatalf("span %d score %v want %v", i, got[i].Score, want[i].Score)
		}
	}
}
