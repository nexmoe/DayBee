package desktop

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// VAD's active-speech duration can include hangover that is trimmed from its
// returned segments. Reproduce that difference without a native model fixture.
type enrollmentTrimmedVAD struct {
	pending, ready []float32
	flushed        bool
	moreSpeech     bool
	voiced         bool
}

func (v *enrollmentTrimmedVAD) Accept(samples []float32) {
	v.voiced = len(samples) > 0 && samples[0] != 0
	if v.voiced && (!v.flushed || v.moreSpeech) {
		v.pending = append(v.pending, samples...)
	}
}
func (v *enrollmentTrimmedVAD) Detected() bool {
	return v.voiced && (!v.flushed || v.moreSpeech)
}
func (v *enrollmentTrimmedVAD) Pop() asr.Speech {
	samples := v.ready
	v.ready = nil
	return asr.Speech{Samples: samples}
}
func (v *enrollmentTrimmedVAD) Flush() {
	samples := v.pending
	if !v.flushed {
		samples = samples[:max(0, len(samples)-audio.SampleRate/4)]
	}
	v.ready, v.pending, v.flushed = samples, nil, true
}
func (v *enrollmentTrimmedVAD) Reset() {}
func (v *enrollmentTrimmedVAD) Close() {}

func TestVoiceprintEnrollmentWaitsForCollectedSpeech(t *testing.T) {
	for _, moreSpeech := range []bool{true, false} {
		name := "continues-speaking"
		if !moreSpeech {
			name = "silence-after-trimming"
		}
		t.Run(name, func(t *testing.T) {
			v, _, closed, active := enrollmentHarness(t, true, true)
			v.timeout = 500 * time.Millisecond
			v.newVAD = func() (asr.VAD, error) {
				return &enrollmentTrimmedVAD{moreSpeech: moreSpeech}, nil
			}
			const targetSeconds = 1.0
			take, err := v.collect(context.Background(), 0, targetSeconds, false)
			samples := take.samples
			if moreSpeech {
				if err != nil || len(samples) < int(targetSeconds*audio.SampleRate) {
					t.Fatalf("collector stopped before enough speech was returned: %d samples, %v", len(samples), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "Not enough speech was detected") {
				t.Fatalf("silence must time out instead of accepting a short sample: %v", err)
			}
			if active.Load() != 0 || closed.Load() != 1 {
				t.Fatal("collection retained the microphone")
			}
		})
	}
}

func TestCollectRejectsNonFinitePCMBeforeEmbedding(t *testing.T) {
	for _, sample := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		v, profiles, closed, active := enrollmentHarness(t, true, true)
		v.newCapture = func() (enrollmentCapture, error) {
			return &enrollmentTestCapture{wave: []float32{sample}, closed: closed, active: active}, nil
		}
		if err := v.Start(); err != nil {
			t.Fatal(err)
		}
		waitEnrollment(t, v, protocol.VoiceprintError)
		message := ""
		if text := v.Snapshot().Error; text != nil {
			message = *text
		}
		v.Cancel()
		if message != voiceprintErrInvalid || profiles.embedded.Load() != 0 || active.Load() != 0 {
			t.Fatalf("invalid sample %g was embedded: %q", sample, message)
		}
	}
}
