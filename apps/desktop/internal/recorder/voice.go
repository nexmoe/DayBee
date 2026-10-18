package recorder

import (
	"math"
	"sync"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

const relaxGap = 2 * time.Second

// voiceMatcher is the speaker profile used by the legacy one-chunk path.
// *voiceprint.Manager satisfies it. Tests substitute a fake.
type voiceMatcher interface {
	Snapshot() *voiceprint.Profile
	Enrolled() bool
	Match(profile *voiceprint.Profile, samples []float32, rate int, ref *voiceprint.Reference) (string, float32)
}

// spanMatcher returns matching evidence within one captured utterance.
// Confidence intervals do not define ASR input boundaries.
// Fakes that only implement voiceMatcher keep the old whole-chunk Match.
type spanMatcher interface {
	MatchSpansRelaxed(profile *voiceprint.Profile, samples []float32, rate int, ref *voiceprint.Reference, relax bool) []voiceprint.IdentitySpan
}

// contextSpanMatcher is the realtime runtime API. Batch labeling does not use it.
type contextSpanMatcher interface {
	MatchSpansContext(profile *voiceprint.Profile, samples []float32, rate int, ref *voiceprint.Reference, context voiceprint.EnrollmentContext, relax bool) []voiceprint.IdentitySpan
}

// voiceRuntime resets in-memory learning when the live device or AEC changes.
type voiceRuntime interface {
	ResetRuntime(voiceprint.EnrollmentContext)
}

// profileCompatible is A's device/AEC check. Tests replace it and restore it.
var profileCompatible = voiceprint.Compatible

func matchVoice(matcher voiceMatcher, profile *voiceprint.Profile, samples []float32, ref *voiceprint.Reference) (identity string, score float32) {
	if matcher == nil || profile == nil || len(profile.Embeddings) == 0 {
		return "", 0
	}
	defer func() {
		if recover() != nil {
			identity, score = "", 0
		}
	}()
	identity, score = matcher.Match(profile, samples, audio.SampleRate, ref)
	if identity != "me" {
		return "", score
	}
	return identity, score
}

// spanMeAllowed drops the badge when a calibrated profile has no stored
// context, or when Compatible rejects the frozen device and echo setting.
// Legacy profiles without the new metadata keep their labels.
func spanMeAllowed(profile *voiceprint.Profile, explicit bool, ctx voiceprint.EnrollmentContext) bool {
	if profile == nil || len(profile.Embeddings) == 0 {
		return false
	}
	if !explicit {
		return profile.Version < 3
	}
	return profileCompatible(profile, ctx)
}

// referenceSilent is true only for a fully observed reference whose RMS is at or
// below -50 dBFS. Nil, empty, and incomplete coverage are not confirmed silence.
func referenceSilent(ref *voiceprint.Reference) bool {
	if ref == nil || !ref.Complete || len(ref.Samples) == 0 {
		return false
	}
	var sum float64
	for _, sample := range ref.Samples {
		value := float64(sample)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		sum += value * value
	}
	rms := math.Sqrt(sum / float64(len(ref.Samples)))
	if rms == 0 {
		return true
	}
	return 20*math.Log10(rms) <= -50
}

// voiceAnchor remembers the latest unrelaxed high-confidence me.
// A me produced by relaxation clears it and cannot seed the next chunk.
type voiceAnchor struct {
	mu      sync.Mutex
	ready   bool
	session string
	device  string
	end     time.Time
}

func (a *voiceAnchor) allow(session, device string, start time.Time, ref *voiceprint.Reference) bool {
	if a == nil || session == "" || !referenceSilent(ref) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.ready || a.session != session || a.device != device {
		return false
	}
	gap := start.Sub(a.end)
	return gap >= 0 && gap < relaxGap
}

func (a *voiceAnchor) observe(session, device string, end time.Time, ref *voiceprint.Reference, relaxed, highMe bool) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if relaxed || !highMe || !referenceSilent(ref) || session == "" || end.IsZero() {
		a.ready = false
		return
	}
	a.ready = true
	a.session = session
	a.device = device
	a.end = end
}

func (a *voiceAnchor) clear() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.ready = false
	a.mu.Unlock()
}

func voiceAnchorFor(e *Engine) *voiceAnchor {
	if e == nil {
		return nil
	}
	return &e.voiceAnchor
}
