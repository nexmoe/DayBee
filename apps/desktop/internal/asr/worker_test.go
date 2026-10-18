package asr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRecognizer struct {
	decode func([]float32, int) (Result, error)
	closed atomic.Int64
}

func (f *fakeRecognizer) Decode(s []float32, r int) (Result, error) { return f.decode(s, r) }
func (f *fakeRecognizer) Close()                                    { f.closed.Add(1) }
func job(ctx context.Context, id int, done func(Result, error)) Job {
	return Job{Context: ctx, Prepare: func() ([]float32, int, error) { return []float32{float32(id)}, 16000, nil }, Complete: done}
}
func TestLivePriorityBoundedQueueAndNativeRelease(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{}, 1)
	unblock := make(chan struct{})
	order := make(chan int, 20)
	rec := &fakeRecognizer{decode: func(s []float32, _ int) (Result, error) {
		if s[0] == 0 {
			entered <- struct{}{}
			<-unblock
		}
		order <- int(s[0])
		return Result{}, nil
	}}
	w := NewWorker(ctx, func() (Recognizer, error) { return rec, nil })
	defer w.Close()
	if err := w.SubmitBatch(job(ctx, 0, nil)); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := w.SubmitBatch(job(ctx, 9, nil)); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		if !w.SubmitLive(job(ctx, i, nil)) {
			t.Fatal("queue rejected early")
		}
	}
	if w.SubmitLive(job(ctx, 7, nil)) {
		t.Fatal("queue exceeded six segments")
	}
	close(unblock)
	for _, want := range []int{0, 1, 2, 3, 4, 5, 6, 9} {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("priority: got %d want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("worker stalled")
		}
	}
	w.ReleaseWhenIdle()
	deadline := time.Now().Add(time.Second)
	for rec.closed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if rec.closed.Load() != 1 {
		t.Fatal("recognizer not released")
	}
}
func TestShutdownSubmissionRaceCompletesAcceptedJobs(t *testing.T) {
	ctx := context.Background()
	w := NewWorker(ctx, func() (Recognizer, error) {
		return &fakeRecognizer{decode: func([]float32, int) (Result, error) { return Result{}, nil }}, nil
	})
	var accepted, completed atomic.Int64
	var submissions sync.WaitGroup
	for i := 0; i < 8; i++ {
		submissions.Add(1)
		go func() {
			defer submissions.Done()
			for j := 0; j < 500; j++ {
				if w.SubmitLive(job(ctx, j, func(Result, error) { completed.Add(1) })) {
					accepted.Add(1)
				}
			}
		}()
	}
	closed := make(chan struct{})
	go func() { w.Close(); close(closed) }()
	submissions.Wait()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown deadlocked")
	}
	if accepted.Load() != completed.Load() || w.Busy() {
		t.Fatal("accepted jobs stranded", accepted.Load(), completed.Load())
	}
}

func TestModelChangeClosesPreviousRecognizerBeforeNextDecode(t *testing.T) {
	var key atomic.Value
	key.Store("qwen")
	var previous *fakeRecognizer
	created := 0
	w := NewKeyedWorker(context.Background(), func() string { return key.Load().(string) }, func() (Recognizer, error) {
		if previous != nil && previous.closed.Load() != 1 {
			t.Error("loaded two models at the same time")
		}
		name := key.Load().(string)
		previous = &fakeRecognizer{decode: func([]float32, int) (Result, error) { return Result{Text: name}, nil }}
		created++
		return previous, nil
	})
	defer w.Close()
	for _, name := range []string{"qwen", "qwen", "firered", "firered", "qwen"} {
		key.Store(name)
		done := make(chan Result, 1)
		if err := w.SubmitBatch(job(context.Background(), 1, func(result Result, err error) {
			if err != nil {
				t.Error(err)
			}
			done <- result
		})); err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-done:
			if result.Text != name {
				t.Fatal("decoded with stale model", result.Text, name)
			}
		case <-time.After(time.Second):
			t.Fatal("model switch stalled")
		}
	}
	if created != 3 {
		t.Fatal("unchanged model was unnecessarily reloaded", created)
	}
}

func TestDelayedReleaseDoesNotUnloadNewModel(t *testing.T) {
	var key atomic.Value
	key.Store("qwen")
	entered, unblock := make(chan struct{}), make(chan struct{})
	var created atomic.Int64
	var newest *fakeRecognizer
	w := NewKeyedWorker(context.Background(), func() string { return key.Load().(string) }, func() (Recognizer, error) {
		created.Add(1)
		name := key.Load().(string)
		newest = &fakeRecognizer{decode: func([]float32, int) (Result, error) {
			if name == "qwen" {
				close(entered)
				<-unblock
			}
			return Result{Text: name}, nil
		}}
		return newest, nil
	})
	defer w.Close()
	w.SubmitLive(job(context.Background(), 1, nil))
	<-entered
	w.ReleaseWhenIdle()
	key.Store("firered")
	done := make(chan Result, 2)
	w.SubmitLive(job(context.Background(), 2, func(result Result, _ error) { done <- result }))
	close(unblock)
	<-done
	deadline := time.Now().Add(time.Second)
	for len(w.release) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(w.release) != 0 {
		t.Fatal("release request stalled")
	}
	w.SubmitLive(job(context.Background(), 3, func(result Result, _ error) { done <- result }))
	if result := <-done; result.Text != "firered" || created.Load() != 2 || newest.closed.Load() != 0 {
		t.Fatal("old release request unloaded the new model")
	}
}

func TestSpansDecodeSeriallyWithoutRequeue(t *testing.T) {
	ctx := context.Background()
	var decodes atomic.Int32
	var mu sync.Mutex
	var sizes []int
	rec := &fakeRecognizer{decode: func(samples []float32, _ int) (Result, error) {
		decodes.Add(1)
		mu.Lock()
		sizes = append(sizes, len(samples))
		mu.Unlock()
		return Result{Text: "段"}, nil
	}}
	w := NewWorker(ctx, func() (Recognizer, error) { return rec, nil })
	done := make(chan error, 1)
	if !w.SubmitLive(Job{Context: ctx, Spans: func(decode func([]float32, int) (Result, error)) error {
		for _, n := range []int{4, 5, 6} {
			if _, err := decode(make([]float32, n), 16000); err != nil {
				return err
			}
		}
		return nil
	}, Complete: func(_ Result, err error) { done <- err }}) {
		t.Fatal("one span job was rejected")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("span job deadlocked")
	}
	w.Close()
	if decodes.Load() != 3 || len(sizes) != 3 || sizes[0] != 4 || sizes[1] != 5 || sizes[2] != 6 {
		t.Fatalf("decodes = %d sizes = %v", decodes.Load(), sizes)
	}
}

func TestSpansCancelBetweenDecodesThenClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var decodes atomic.Int32
	rec := &fakeRecognizer{decode: func([]float32, int) (Result, error) {
		decodes.Add(1)
		return Result{Text: "段"}, nil
	}}
	w := NewWorker(context.Background(), func() (Recognizer, error) { return rec, nil })
	done := make(chan error, 1)
	if !w.SubmitLive(Job{Context: ctx, Spans: func(decode func([]float32, int) (Result, error)) error {
		if _, err := decode([]float32{1}, 16000); err != nil {
			return err
		}
		cancel()
		_, err := decode([]float32{1, 2}, 16000)
		return err
	}, Complete: func(_ Result, err error) { done <- err }}) {
		t.Fatal("span job was rejected")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel was ignored")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled span job deadlocked")
	}
	closed := make(chan struct{})
	go func() { w.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("close deadlocked")
	}
	if decodes.Load() != 1 {
		t.Fatalf("decoded %d times after cancel", decodes.Load())
	}
}

func TestSpanDecodeDropsResultWhenContextEndsDuringDecode(t *testing.T) {
	for _, name := range []string{"job", "worker"} {
		t.Run(name, func(t *testing.T) {
			parent, stopWorker := context.WithCancel(context.Background())
			jobCtx, stopJob := context.WithCancel(context.Background())
			rec := &fakeRecognizer{decode: func([]float32, int) (Result, error) {
				if name == "job" {
					stopJob()
				} else {
					stopWorker()
				}
				return Result{Text: "不应保留"}, nil
			}}
			w := NewWorker(parent, func() (Recognizer, error) { return rec, nil })
			done := make(chan error, 1)
			var got Result
			if !w.SubmitLive(Job{Context: jobCtx, Spans: func(decode func([]float32, int) (Result, error)) error {
				result, err := decode([]float32{1}, 16000)
				got = result
				return err
			}, Complete: func(_ Result, err error) { done <- err }}) {
				t.Fatal("span job was rejected")
			}
			select {
			case err := <-done:
				if err == nil || got.Text != "" {
					t.Fatalf("kept %q after %v", got.Text, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("span decode did not observe cancellation")
			}
			w.Close()
		})
	}
}
