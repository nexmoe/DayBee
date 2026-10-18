package recorder

import (
	"sync"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/voiceprint"
)

const echoKeep = 30 * audio.SampleRate

// echoReference is the latest ~30s of system PCM on an absolute sample clock.
// Sample seq lives at buf[seq%echoKeep]. The retained range is the last echoKeep
// samples ending at writeSeq.
type echoReference struct {
	mu       sync.Mutex
	buf      []float32
	covered  []bool
	origin   time.Time
	writeSeq int64
	ready    bool
	anchor   time.Time
	fed      int
	epoch    uint64
}

func newEchoReference() *echoReference {
	return &echoReference{buf: make([]float32, echoKeep), covered: make([]bool, echoKeep)}
}

func (r *echoReference) AppendSegment(segmentStart time.Time, samples []float32) {
	if r == nil || len(samples) == 0 || segmentStart.IsZero() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !segmentStart.Equal(r.anchor) {
		r.anchor = segmentStart
		r.fed = 0
	}
	at := echoAt(segmentStart, r.fed)
	r.fed += len(samples)
	r.appendLocked(at, samples)
}

func (r *echoReference) AppendAt(at time.Time, samples []float32) {
	if r == nil || len(samples) == 0 || at.IsZero() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appendLocked(at, samples)
}

func (r *echoReference) appendLocked(at time.Time, pcm []float32) {
	if len(pcm) == 0 {
		return
	}
	if extra := len(pcm) - echoKeep; extra > 0 {
		at = echoAt(at, extra)
		pcm = pcm[extra:]
	}
	if !r.ready {
		r.origin = at
		r.ready = true
		r.writeSeq = 0
	}
	start := echoOffset(r.origin, at)
	if start < 0 {
		skip := int(-start)
		if skip >= len(pcm) {
			return
		}
		pcm = pcm[skip:]
		start = 0
	}
	end := start + int64(len(pcm))
	finalEnd := end
	if r.writeSeq > finalEnd {
		finalEnd = r.writeSeq
	}
	keepFrom := finalEnd - int64(echoKeep)
	if keepFrom < 0 {
		keepFrom = 0
	}
	if end <= keepFrom {
		return
	}
	if start > r.writeSeq {
		zeroFrom := r.writeSeq
		if zeroFrom < keepFrom {
			zeroFrom = keepFrom
		}
		for seq := zeroFrom; seq < start; seq++ {
			slot := int(seq % int64(echoKeep))
			r.buf[slot] = 0
			r.covered[slot] = false
		}
	}
	for i, sample := range pcm {
		seq := start + int64(i)
		if seq < keepFrom {
			continue
		}
		slot := int(seq % int64(echoKeep))
		r.buf[slot] = sample
		r.covered[slot] = true
	}
	if end > r.writeSeq {
		r.writeSeq = end
	}
}

// Slice returns samples covering [start, end), zero-padded to the exact length.
// It returns nil when the request does not overlap audio still in the ring.
func (r *echoReference) Slice(start, end time.Time) []float32 {
	samples, _ := r.SliceWithCoverage(start, end)
	return samples
}

// SliceWithCoverage reports whether every returned sample was observed.
// A gap, an expired slot, or a partial overlap is padded with zeros and is not complete.
func (r *echoReference) SliceWithCoverage(start, end time.Time) ([]float32, bool) {
	if r == nil || !end.After(start) {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ready || r.writeSeq <= 0 {
		return nil, false
	}
	want := echoOffset(start, end)
	if want <= 0 {
		return nil, false
	}
	rel := echoOffset(r.origin, start)
	validFrom := r.writeSeq - int64(echoKeep)
	if validFrom < 0 {
		validFrom = 0
	}
	if rel >= r.writeSeq || rel+want <= validFrom {
		return nil, false
	}
	out := make([]float32, want)
	complete := true
	for i := int64(0); i < want; i++ {
		seq := rel + i
		if seq < validFrom || seq >= r.writeSeq || !r.covered[int(seq%int64(echoKeep))] {
			complete = false
			continue
		}
		out[i] = r.buf[int(seq%int64(echoKeep))]
	}
	return out, complete
}

func (r *echoReference) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearLocked()
}

func (r *echoReference) ResetIf(epoch uint64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch {
		return
	}
	r.clearLocked()
}

func (r *echoReference) Epoch() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.epoch
}

func (r *echoReference) clearLocked() {
	r.writeSeq = 0
	r.ready = false
	r.origin = time.Time{}
	r.anchor = time.Time{}
	r.fed = 0
	clear(r.covered)
	r.epoch++
}

func (e *Engine) resetEcho() {
	if e != nil && e.echoRef != nil {
		e.echoRef.Reset()
	}
}

func (e *Engine) captureSystemEcho(stream *audioStream, samples []float32) {
	if e == nil || e.echoRef == nil || stream == nil || stream.segment == nil || len(samples) == 0 {
		return
	}
	e.echoRef.AppendSegment(stream.segment.start, samples)
}

func (e *Engine) liveReference(start, end time.Time) *voiceprint.Reference {
	if e == nil || e.echoRef == nil {
		return nil
	}
	failed := e.refFailedEpoch.Load()
	if failed != 0 && failed == e.refEpoch.Load() {
		return nil
	}
	samples, complete := e.echoRef.SliceWithCoverage(start, end)
	if samples == nil {
		return nil
	}
	return &voiceprint.Reference{Samples: samples, Complete: complete}
}

func echoAt(start time.Time, samples int) time.Time {
	neg := samples < 0
	if neg {
		samples = -samples
	}
	sec, rem := samples/audio.SampleRate, samples%audio.SampleRate
	d := time.Duration(sec)*time.Second + time.Duration(rem)*time.Second/audio.SampleRate
	if neg {
		return start.Add(-d)
	}
	return start.Add(d)
}

func echoOffset(from, to time.Time) int64 {
	if to.Before(from) {
		return -echoOffset(to, from)
	}
	d := to.Sub(from)
	sec, rem := int64(d/time.Second), int64(d%time.Second)
	return sec*int64(audio.SampleRate) + rem*int64(audio.SampleRate)/int64(time.Second)
}
