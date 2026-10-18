package asr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueuedJobsKeepTheirModelWhileSelectionChanges(t *testing.T) {
	var selected atomic.Value
	selected.Store("qwen")
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate) }) }
	var loaded atomic.Int64
	worker := NewModelWorker(context.Background(), func() string { return selected.Load().(string) }, func(model string) (Recognizer, error) {
		if loaded.Add(1) != 1 {
			t.Error("two recognizers loaded at once")
		}
		return &modelRecognizer{model: model, loaded: &loaded, decode: func() {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-gate
		}}, nil
	})
	defer worker.Close()
	defer unblock()
	done := make(chan string, 4)
	submit := func(model string, spans bool) {
		t.Helper()
		next := job(context.Background(), 1, func(result Result, err error) {
			if err != nil {
				t.Error(err)
			}
			if !spans {
				done <- result.Text
			}
		})
		next.ModelPath = model
		if spans {
			next.Spans = func(decode func([]float32, int) (Result, error)) error {
				result, err := decode([]float32{1}, 16000)
				done <- result.Text
				return err
			}
		}
		if !worker.SubmitLive(next) {
			t.Fatal("live job rejected")
		}
	}
	submit("qwen", false)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("initial inference did not start")
	}
	submit("qwen", true)
	worker.ReleaseWhenIdle()
	selected.Store("firered")
	submit("firered", false)
	selected.Store("sensevoice")
	submit("qwen", false)
	unblock()
	for _, want := range []string{"qwen", "qwen", "firered", "qwen"} {
		select {
		case got := <-done:
			if got != want {
				t.Fatalf("decoded with %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("model switch stranded queued speech")
		}
	}
}

type modelRecognizer struct {
	model  string
	loaded *atomic.Int64
	decode func()
}

func (r *modelRecognizer) Decode([]float32, int) (Result, error) {
	r.decode()
	return Result{Text: r.model}, nil
}

func (r *modelRecognizer) Close() { r.loaded.Add(-1) }
