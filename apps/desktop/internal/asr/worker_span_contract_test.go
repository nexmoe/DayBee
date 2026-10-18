package asr

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestFinalSpanCancelledDuringNativeDecode(t *testing.T) {
	for _, scope := range []string{"job", "worker"} {
		t.Run(scope, func(t *testing.T) {
			parent, cancelWorker := context.WithCancel(context.Background())
			defer cancelWorker()
			ctx, cancelJob := context.WithCancel(context.Background())
			defer cancelJob()
			cancel := cancelJob
			if scope == "worker" {
				cancel = cancelWorker
			}
			rec := &fakeRecognizer{decode: func([]float32, int) (Result, error) {
				cancel()
				return Result{Text: "late result"}, nil
			}}
			worker := NewWorker(parent, func() (Recognizer, error) { return rec, nil })
			defer worker.Close()
			done := make(chan error, 1)
			var completed atomic.Int32
			if !worker.SubmitLive(Job{Context: ctx, Spans: func(decode func([]float32, int) (Result, error)) error {
				result, err := decode([]float32{1}, 16000)
				if result.Text != "" {
					t.Error("cancelled decode exposed a late transcript")
				}
				return err
			}, Complete: func(_ Result, err error) {
				completed.Add(1)
				done <- err
			}}) {
				t.Fatal("job rejected before cancellation")
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("completion error = %v; want cancellation", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled job did not complete")
			}
			worker.Close()
			if completed.Load() != 1 || worker.Busy() || rec.closed.Load() != 1 {
				t.Fatal("cancelled job or recognizer stranded")
			}
		})
	}
}
