package voiceprint

import (
	"math"
	"math/rand/v2"
	"sort"
	"testing"
)

const (
	runtimeEvalSeed        = 20261012
	runtimeEvalHoldSeconds = 8
	runtimeEvalGroupSec    = 8
	runtimeEvalDelayMs     = 80
	runtimeEvalWet         = float32(0.5)
	runtimeEvalOwnGain     = float32(0.5)
	runtimeEvalMixGain     = float32(0.5)
	runtimeEvalSNR         = 15.0
)

// runtimeEvalContext is the enrolled device. Mismatch trials use a different value.
var runtimeEvalContext = EnrollmentContext{Device: "eval:mic-a", EchoCancellation: false}

type runtimeEvalUtt struct {
	Name  string
	Index int
	PCM   []float32
}

type runtimeEvalSpeaker struct {
	ID              string
	Test            []float32
	TestFiles       []string
	EnrollmentFiles [][]string
	GroupSeconds    []float64
	SkippedShort    int
	Split           string
	Audio           string
	Profile         *Profile
	Error           string
	GenuineMin      float64
	GenuineMax      float64
	ImpostorMax     float64
	groups          [3][]float32
}

type runtimeEvalRegion struct {
	Start, End int
	Truth      string
}

type runtimeEvalClip struct {
	Kind, Truth, Bucket, Other, Note string
	Samples                          []float32
	Ref                              *Reference
	Context                          EnrollmentContext
	ExpectNoMe                       bool
	Expected                         []runtimeEvalBoundary
	Regions                          []runtimeEvalRegion
}

func runtimeEvalHoldout(id string, files []runtimeEvalUtt) (test, rest []runtimeEvalUtt, samples int, ok bool) {
	order := append([]runtimeEvalUtt(nil), files...)
	if id == "SSB0700" {
		sort.SliceStable(order, func(i, j int) bool {
			if len(order[i].PCM) == len(order[j].PCM) {
				return order[i].Index < order[j].Index
			}
			return len(order[i].PCM) < len(order[j].PCM)
		})
	} else {
		sort.SliceStable(order, func(i, j int) bool { return order[i].Index > order[j].Index })
	}
	need := runtimeEvalHoldSeconds * voiceSampleRate
	for len(order) > 0 && samples < need {
		test = append(test, order[0])
		samples += len(order[0].PCM)
		order = order[1:]
	}
	return test, order, samples, samples >= need
}

func runtimeEvalGroups(files []runtimeEvalUtt) (groups [3][]runtimeEvalUtt, samples [3]int, ok bool) {
	order := append([]runtimeEvalUtt(nil), files...)
	sort.SliceStable(order, func(i, j int) bool {
		if len(order[i].PCM) == len(order[j].PCM) {
			return order[i].Index < order[j].Index
		}
		return len(order[i].PCM) > len(order[j].PCM)
	})
	for _, file := range order {
		group := 0
		for j := 1; j < 3; j++ {
			if samples[j] < samples[group] {
				group = j
			}
		}
		groups[group] = append(groups[group], file)
		samples[group] += len(file.PCM)
	}
	ok = true
	need := runtimeEvalGroupSec * voiceSampleRate
	for _, n := range samples {
		if n < need {
			ok = false
		}
	}
	return groups, samples, ok
}

func runtimeEvalConcat(files []runtimeEvalUtt) []float32 {
	var out []float32
	for _, file := range files {
		out = append(out, file.PCM...)
	}
	return out
}

func runtimeEvalScale(in []float32, gain float32) []float32 {
	out := make([]float32, len(in))
	for i, sample := range in {
		out[i] = gain * sample
	}
	return out
}

func runtimeEvalPinkAtSNR(signal []float32, snrDB float64, seed uint64) []float32 {
	rng := rand.New(rand.NewPCG(seed, 0xE3A11))
	noise := evalPinkNoise(rng, len(signal))
	var signalPower, noisePower float64
	for _, sample := range signal {
		signalPower += float64(sample) * float64(sample)
	}
	for _, sample := range noise {
		noisePower += float64(sample) * float64(sample)
	}
	signalPower /= float64(len(signal))
	noisePower /= float64(len(noise))
	scale := 0.0
	if signalPower > 0 && noisePower > 0 {
		scale = math.Sqrt(signalPower / (math.Pow(10, snrDB/10) * noisePower))
	}
	out := make([]float32, len(signal))
	for i := range out {
		out[i] = signal[i] + float32(scale*float64(noise[i]))
	}
	return out
}

// runtimeEvalDelayedReverb restarts at seed. The room is evalRoomIR (about 200 ms),
// then a fixed 80 ms delay and a wet gain of 0.5. No extra noise is added.
func runtimeEvalDelayedReverb(signal []float32, seed uint64) []float32 {
	rng := rand.New(rand.NewPCG(seed, 0xE3A11))
	wet := evalConvolve(signal, evalRoomIR(rng))
	delay := runtimeEvalDelayMs * voiceSampleRate / 1000
	out := make([]float32, delay+len(wet))
	for i, sample := range wet {
		out[delay+i] = runtimeEvalWet * sample
	}
	return out
}

func runtimeEvalPrefix(samples []float32, n int) ([]float32, bool) {
	if n <= 0 || len(samples) < n {
		return nil, false
	}
	return append([]float32(nil), samples[:n]...), true
}

func runtimeEvalMix(me, other []float32, gain float32) []float32 {
	n := len(me)
	if len(other) < n {
		n = len(other)
	}
	out := make([]float32, n)
	for i := range out {
		out[i] = me[i] + gain*other[i]
	}
	return out
}

func runtimeEvalRef(samples []float32) *Reference {
	return &Reference{Samples: append([]float32(nil), samples...), Complete: true}
}

// runtimeEvalPaddedRef keeps the dry playback at sample 0 and fills the rest
// with zeros. Those zeros are synthetic silence for a finished system stream.
func runtimeEvalPaddedRef(signal []float32, n int) *Reference {
	if n < len(signal) {
		n = len(signal)
	}
	samples := make([]float32, n)
	copy(samples, signal)
	return &Reference{Samples: samples, Complete: true}
}

func runtimeEvalClips(self runtimeEvalSpeaker, all []runtimeEvalSpeaker) []runtimeEvalClip {
	var clips []runtimeEvalClip
	me := append([]float32(nil), self.Test...)
	add := func(clip runtimeEvalClip) {
		if len(clip.Samples) == 0 {
			return
		}
		if clip.Context.Device == "" {
			clip.Context = runtimeEvalContext
		}
		clips = append(clips, clip)
	}
	quiet := append([]float32(nil), me...)
	add(runtimeEvalClip{Kind: "clean_quiet_reference", Truth: "me", Samples: quiet,
		Ref:  &Reference{Samples: make([]float32, len(quiet)), Complete: true},
		Note: "same raw test as clean_me; equal-length digital silence with Complete=true. Later clips use a fresh snapshot. A zero adaptation count is reported as measured."})
	add(runtimeEvalClip{Kind: "clean_me", Truth: "me", Samples: me, Note: "no reference; this path cannot learn"})
	var others []runtimeEvalSpeaker
	for _, speaker := range all {
		if speaker.ID != self.ID && len(speaker.Test) > 0 {
			others = append(others, speaker)
		}
	}
	for _, other := range others {
		pcm := append([]float32(nil), other.Test...)
		add(runtimeEvalClip{Kind: "other", Truth: "not_me", Other: other.ID, Samples: pcm})
	}
	add(runtimeEvalClip{Kind: "own_video", Truth: "not_me", Samples: runtimeEvalScale(me, runtimeEvalOwnGain), Ref: runtimeEvalRef(me),
		Note: "mic is the raw test at gain 0.5; system reference is the same raw test"})
	wet := runtimeEvalDelayedReverb(me, runtimeEvalSeed)
	add(runtimeEvalClip{Kind: "delayed_reverb", Truth: "not_me", Samples: wet, Ref: runtimeEvalPaddedRef(me, len(wet)),
		Note: "dry system playback still starts at sample 0. The tail is known synthetic silence padded to the mic length, not a coverage gap."})
	if len(others) > 0 {
		other := others[0]
		mixed := runtimeEvalMix(me, other.Test, runtimeEvalMixGain)
		add(runtimeEvalClip{Kind: "overlap", Truth: "mixed", Other: other.ID, Samples: mixed, Ref: runtimeEvalRef(other.Test[:len(mixed)]),
			Note: "truth: enrolled voice is present for the whole mix; excluded from impostor FAR"})
		if me4, ok := runtimeEvalPrefix(me, 4*voiceSampleRate); ok {
			if other4, ok := runtimeEvalPrefix(other.Test, 4*voiceSampleRate); ok {
				seq := append(append(append([]float32{}, me4...), other4...), me4...)
				at4, at8 := 4*voiceSampleRate, 8*voiceSampleRate
				add(runtimeEvalClip{Kind: "sequence", Truth: "sequence", Other: other.ID, Samples: seq,
					Note: "4s me + 4s other + 4s me; the middle region is not in the other-kind FAR",
					Regions: []runtimeEvalRegion{
						{Start: 0, End: at4, Truth: "me"},
						{Start: at4, End: at8, Truth: "not_me"},
						{Start: at8, End: 12 * voiceSampleRate, Truth: "me"},
					},
					Expected: []runtimeEvalBoundary{
						{At: at4, Left: spanMe, Right: spanNotMe},
						{At: at8, Left: spanNotMe, Right: spanMe},
					}})
			}
		}
	}
	add(runtimeEvalClip{Kind: "noise_15db", Truth: "me", Samples: runtimeEvalPinkAtSNR(me, runtimeEvalSNR, runtimeEvalSeed),
		Note: "pink noise at 15 dB SNR versus the raw test"})
	add(runtimeEvalClip{Kind: "device_mismatch", Truth: "contract", ExpectNoMe: true, Samples: append([]float32(nil), me...),
		Context: EnrollmentContext{Device: "eval:mic-b"}, Note: "compatible device is required; me is a functional failure"})
	add(runtimeEvalClip{Kind: "aec_mismatch", Truth: "contract", ExpectNoMe: true, Samples: append([]float32(nil), me...),
		Context: EnrollmentContext{Device: runtimeEvalContext.Device, EchoCancellation: true}})
	for _, bucket := range []struct {
		label   string
		seconds float64
	}{{"0.5", 0.5}, {"1", 1}, {"1.5", 1.5}, {"2.5", 2.5}} {
		pcm, ok := runtimeEvalPrefix(me, int(bucket.seconds*float64(voiceSampleRate)))
		if !ok {
			continue
		}
		add(runtimeEvalClip{Kind: "short", Truth: "me", Bucket: bucket.label, Samples: pcm})
	}
	return clips
}

func TestRuntimeEvalUtteranceSplit(t *testing.T) {
	long := make([]float32, runtimeEvalHoldSeconds*voiceSampleRate)
	files := []runtimeEvalUtt{
		{Name: "short", Index: 1, PCM: make([]float32, 100)},
		{Name: "b", Index: 2, PCM: long},
		{Name: "c", Index: 3, PCM: long},
		{Name: "d", Index: 4, PCM: long},
		{Name: "e", Index: 5, PCM: long},
	}
	test, rest, samples, ok := runtimeEvalHoldout("SSB0700", files)
	if !ok || samples < len(long) || test[0].Name != "short" || test[1].Name != "b" || len(rest) != 3 {
		t.Fatalf("SSB0700 shortest-first test=%v rest=%d samples=%d", namesOf(test), len(rest), samples)
	}
	groups, lengths, groupsOK := runtimeEvalGroups(rest)
	if !groupsOK {
		t.Fatal("balanced groups were rejected")
	}
	for i, group := range groups {
		if len(group) != 1 || lengths[i] != len(long) {
			t.Fatalf("group %d has %d files", i, len(group))
		}
	}
	test, rest, samples, ok = runtimeEvalHoldout("SSB0544", files[1:])
	if !ok || len(test) != 1 || test[0].Index != 5 || samples != len(long) || len(rest) != 3 {
		t.Fatalf("index holdout test=%v rest=%d", namesOf(test), len(rest))
	}
	_, _, _, ok = runtimeEvalHoldout("SSB0544", []runtimeEvalUtt{{Name: "tiny", Index: 1, PCM: make([]float32, 1000)}})
	if ok {
		t.Fatal("short corpus counted as an 8s holdout")
	}
	oneGroup := []runtimeEvalUtt{{Name: "only", Index: 1, PCM: long}}
	if _, _, ok = runtimeEvalGroups(oneGroup); ok {
		t.Fatal("one utterance became three groups")
	}
}

func TestRuntimeEvalQuietReferenceLeads(t *testing.T) {
	pcm := make([]float32, runtimeEvalHoldSeconds*voiceSampleRate)
	self := runtimeEvalSpeaker{ID: "SSB0544", Test: pcm}
	other := runtimeEvalSpeaker{ID: "SSB0700", Test: pcm}
	clips := runtimeEvalClips(self, []runtimeEvalSpeaker{self, other})
	first := clips[0]
	if first.Kind != "clean_quiet_reference" || first.Ref == nil || !first.Ref.Complete || len(first.Ref.Samples) != len(first.Samples) || len(first.Samples) != len(pcm) {
		t.Fatalf("quiet reference is not the leading equal-length clip: %+v", first.Kind)
	}
	var plain bool
	for _, clip := range clips[1:] {
		if clip.Kind == "clean_me" && clip.Ref == nil && len(clip.Samples) == len(pcm) {
			plain = true
		}
	}
	if !plain {
		t.Fatal("clean_me no longer keeps the no-reference path")
	}
	for _, clip := range clips {
		if clip.Kind != "delayed_reverb" {
			continue
		}
		if clip.Ref == nil || !clip.Ref.Complete || len(clip.Ref.Samples) != len(clip.Samples) {
			t.Fatalf("reverb reference %d does not cover mic %d", len(clip.Ref.Samples), len(clip.Samples))
		}
	}
}

func TestRuntimeEvalReverbReferenceKeepsPlaybackStart(t *testing.T) {
	signal := []float32{0.2, -0.1, 0.05, 0.01}
	mic := runtimeEvalDelayedReverb(signal, runtimeEvalSeed)
	ref := runtimeEvalPaddedRef(signal, len(mic))
	if len(mic) <= len(signal) || ref == nil || !ref.Complete || len(ref.Samples) != len(mic) {
		t.Fatalf("mic %d ref %d", len(mic), len(ref.Samples))
	}
	for i, sample := range signal {
		if ref.Samples[i] != sample {
			t.Fatalf("playback start moved at %d", i)
		}
	}
	for i := len(signal); i < len(ref.Samples); i++ {
		if ref.Samples[i] != 0 {
			t.Fatalf("tail sample %d is not synthetic silence", i)
		}
	}
}

func namesOf(files []runtimeEvalUtt) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name
	}
	return names
}
