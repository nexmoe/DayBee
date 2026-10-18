package recorder

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func TestEchoReferenceSlicePadGapAndReset(t *testing.T) {
	r := newEchoReference()
	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	r.AppendAt(start, []float32{1, 2, 3})
	sameSamples(t, r.Slice(start, echoAt(start, 3)), []float32{1, 2, 3})
	sameSamples(t, r.Slice(echoAt(start, -1), echoAt(start, 5)), []float32{0, 1, 2, 3, 0, 0})
	if r.Slice(start.Add(time.Minute), start.Add(time.Minute+time.Second)) != nil {
		t.Fatal("slice outside the ring returned samples")
	}
	r.AppendAt(echoAt(start, 5), []float32{9})
	sameSamples(t, r.Slice(start, echoAt(start, 6)), []float32{1, 2, 3, 0, 0, 9})
	r.AppendAt(start.Add(time.Minute), []float32{8})
	if r.Slice(start, echoAt(start, 1)) != nil {
		t.Fatal("audio older than the ring survived a one-minute gap")
	}
	sameSamples(t, r.Slice(start.Add(time.Minute), echoAt(start.Add(time.Minute), 1)), []float32{8})
	epoch := r.Epoch()
	r.Reset()
	if r.Slice(start.Add(time.Minute), echoAt(start.Add(time.Minute), 1)) != nil {
		t.Fatal("reset kept samples")
	}
	r.AppendAt(start, []float32{4})
	r.ResetIf(epoch)
	sameSamples(t, r.Slice(start, echoAt(start, 1)), []float32{4})
	r.ResetIf(r.Epoch())
	if r.Slice(start, echoAt(start, 1)) != nil {
		t.Fatal("matching epoch did not reset")
	}
}

func TestEchoReferenceSegmentClockAndWrap(t *testing.T) {
	r := newEchoReference()
	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	r.AppendSegment(start, []float32{1, 2})
	r.AppendSegment(start, []float32{3})
	sameSamples(t, r.Slice(start, echoAt(start, 3)), []float32{1, 2, 3})
	r.AppendSegment(start.Add(time.Second), []float32{8})
	sameSamples(t, r.Slice(echoAt(start, 3), echoAt(start, 4)), []float32{0})
	sameSamples(t, r.Slice(start.Add(time.Second), echoAt(start.Add(time.Second), 1)), []float32{8})

	ring := newEchoReference()
	block := make([]float32, audio.SampleRate)
	for i := range block {
		block[i] = 0.1
	}
	for sec := 0; sec < 30; sec++ {
		ring.AppendAt(start.Add(time.Duration(sec)*time.Second), block)
	}
	tail := make([]float32, 1000)
	for i := range tail {
		tail[i] = 0.7
	}
	ring.AppendAt(start.Add(30*time.Second), tail)
	if ring.Slice(start, echoAt(start, 1000)) != nil {
		t.Fatal("wrapped ring kept the oldest samples")
	}
	sameSamples(t, ring.Slice(start.Add(30*time.Second), echoAt(start.Add(30*time.Second), 1000)), tail)
	boundary := ring.Slice(echoAt(start, echoKeep-10), echoAt(start, echoKeep+10))
	if len(boundary) != 20 {
		t.Fatalf("boundary len = %d", len(boundary))
	}
	for i := 0; i < 10; i++ {
		if boundary[i] != 0.1 || boundary[i+10] != 0.7 {
			t.Fatalf("boundary[%d] = %v %v", i, boundary[i], boundary[i+10])
		}
	}
}

func TestEchoReferenceConcurrentSlice(t *testing.T) {
	r := newEchoReference()
	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pcm := []float32{0.1, 0.2, 0.3, 0.4}
		for i := 0; i < 200; i++ {
			r.AppendAt(echoAt(start, i*len(pcm)), pcm)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = r.Slice(start, echoAt(start, 800))
		}
	}()
	wg.Wait()
}

func TestConsumeStoresSystemEchoBeforeVAD(t *testing.T) {
	e, _, _, _ := testEngine(t, true, &fakeRuntime{})
	enableSystemAudio(t, e)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	packet := sourcePacket(protocol.SourceSystem, 100, 0.25)
	packet.Samples[0] = 0.5
	var start time.Time
	var got []float32
	if err := e.call(func() error {
		start = e.system.segment.start
		if err := e.consume(packet); err != nil {
			return err
		}
		if err := e.consume(sourcePacket(protocol.SourceMicrophone, 100, 0.9)); err != nil {
			return err
		}
		got = e.echoRef.Slice(start, echoAt(start, len(packet.Samples)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("inactive reference wrote the ring: %v", got)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if e.echoRef.Slice(start, echoAt(start, 100)) != nil {
		t.Fatal("stop left system audio in the echo ring")
	}
}

func TestSystemTrackAlignsSiblingAndRejectsBadFiles(t *testing.T) {
	e, _, assets, root := testEngine(t, true, &fakeRuntime{})
	start := time.Date(2026, 10, 11, 1, 2, 3, 0, time.UTC)
	mic := filepath.Join(root, "daybee_20261011_010203_microphone.wav")
	sys := filepath.Join(root, "daybee_20261011_010203_system.flac")
	if err := os.WriteFile(mic, []byte("m"), 0600); err != nil || os.WriteFile(sys, []byte("s"), 0600) != nil {
		t.Fatal(err)
	}
	ranges := []protocol.RecordingTimeRange{
		{StartedAt: start.Format(time.RFC3339Nano), EndedAt: start.Add(100 * time.Millisecond).Format(time.RFC3339Nano)},
		{StartedAt: start.Add(200 * time.Millisecond).Format(time.RFC3339Nano), EndedAt: start.Add(300 * time.Millisecond).Format(time.RFC3339Nano)},
	}
	first, second := make([]float32, 1600), make([]float32, 1600)
	for i := range first {
		first[i], second[i] = 0.2, 0.4
	}
	if err := config.WriteJSON(stringsTrimExt(sys)+".recording.json", map[string]any{
		"audio_path": sys, "audio_source": protocol.SourceSystem, "captured_ranges": ranges,
	}); err != nil {
		t.Fatal(err)
	}
	assets.read = func(_ context.Context, file string) ([]float32, int, error) {
		if file != sys {
			t.Errorf("read %s", file)
		}
		return append(append([]float32{}, first...), second...), audio.SampleRate, nil
	}
	track := e.systemTrack(context.Background(), mic)
	got := track.reference(speechPiece{samples: make([]float32, 3200), started: start.Add(50 * time.Millisecond).Format(time.RFC3339Nano), ended: start.Add(250 * time.Millisecond).Format(time.RFC3339Nano)})
	if got == nil || len(got.Samples) != 3200 {
		t.Fatalf("reference = %#v", got)
	}
	for i := 0; i < 800; i++ {
		if got.Samples[i] != 0.2 || got.Samples[i+800] != 0 || got.Samples[i+2400] != 0.4 {
			t.Fatalf("aligned[%d] = %v %v %v", i, got.Samples[i], got.Samples[i+800], got.Samples[i+2400])
		}
	}
	if e.systemTrack(context.Background(), filepath.Join(root, "mic.wav")) != nil {
		t.Fatal("unrelated microphone name found a system file")
	}
	assets.read = func(context.Context, string) ([]float32, int, error) { return []float32{1}, audio.SampleRate, nil }
	if e.systemTrack(context.Background(), mic) != nil {
		t.Fatal("misaligned captured ranges produced a reference")
	}
	suffixed := filepath.Join(root, "daybee_20261011_010203_system_42.wav")
	if err := os.WriteFile(suffixed, []byte("s"), 0600); err != nil {
		t.Fatal(err)
	}
	os.Remove(sys)
	assets.read = func(_ context.Context, file string) ([]float32, int, error) {
		if file != suffixed {
			return nil, 0, os.ErrNotExist
		}
		return first, audio.SampleRate, nil
	}
	if err := config.WriteJSON(stringsTrimExt(suffixed)+".recording.json", map[string]any{
		"captured_ranges": []protocol.RecordingTimeRange{ranges[0]},
	}); err != nil {
		t.Fatal(err)
	}
	found := e.systemTrack(context.Background(), mic)
	if found == nil || found.reference(speechPiece{samples: make([]float32, 1600), started: ranges[0].StartedAt, ended: ranges[0].EndedAt}) == nil {
		t.Fatal("suffixed system sibling was ignored")
	}
}

func stringsTrimExt(path string) string {
	return path[:len(path)-len(filepath.Ext(path))]
}

func sameSamples(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %v want %v", i, got[i], want[i])
		}
	}
}
