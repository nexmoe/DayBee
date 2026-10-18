package asr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestIdleRecognizerIsReleasedAndReloadedOnSpeech(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var created atomic.Int64
		var recognizers []*fakeRecognizer
		w := NewWorker(context.Background(), func() (Recognizer, error) {
			created.Add(1)
			r := &fakeRecognizer{decode: func([]float32, int) (Result, error) { return Result{Text: "speech"}, nil }}
			recognizers = append(recognizers, r)
			return r, nil
		})
		defer w.Close()
		synctest.Wait()
		time.Sleep(2 * modelIdleTimeout)
		if created.Load() != 0 {
			t.Fatal("idle worker eagerly loaded a model")
		}
		submit := func() {
			t.Helper()
			done := make(chan Result, 1)
			if !w.SubmitLive(job(context.Background(), 1, func(result Result, err error) {
				if err != nil {
					t.Error(err)
				}
				done <- result
			})) {
				t.Fatal("speech was rejected")
			}
			if result := <-done; result.Text != "speech" {
				t.Fatal("speech was lost", result)
			}
			synctest.Wait()
		}
		submit()
		time.Sleep(modelIdleTimeout / 2)
		submit()
		time.Sleep(modelIdleTimeout / 2)
		synctest.Wait()
		if created.Load() != 1 || recognizers[0].closed.Load() != 0 {
			t.Fatal("ordinary pauses must reuse the warm model and reset the idle deadline")
		}
		time.Sleep(modelIdleTimeout/2 + time.Second)
		synctest.Wait()
		if recognizers[0].closed.Load() != 1 || w.Busy() {
			t.Fatal("idle native model was retained")
		}
		submit()
		if created.Load() != 2 || recognizers[1].closed.Load() != 0 {
			t.Fatal("speech after idle did not reload a usable recognizer")
		}
	})
}

func TestIdleTimeoutNeverReleasesRunningOrQueuedInference(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		var release sync.Once
		unblock := func() { release.Do(func() { close(gate) }) }
		rec := &fakeRecognizer{decode: func([]float32, int) (Result, error) {
			<-gate
			return Result{}, nil
		}}
		w := NewWorker(context.Background(), func() (Recognizer, error) { return rec, nil })
		defer w.Close()
		defer unblock()
		var completed atomic.Int64
		next := job(context.Background(), 1, func(Result, error) { completed.Add(1) })
		if !w.SubmitLive(next) {
			t.Fatal("speech rejected")
		}
		synctest.Wait()
		if !w.SubmitLive(next) || w.SubmitBatch(next) != nil {
			t.Fatal("queued work rejected")
		}
		time.Sleep(3 * modelIdleTimeout)
		synctest.Wait()
		if rec.closed.Load() != 0 || !w.Busy() {
			t.Fatal("released a model with pending speech")
		}
		unblock()
		synctest.Wait()
		if completed.Load() != 3 || rec.closed.Load() != 0 {
			t.Fatal("queued speech was lost or model released before its idle deadline")
		}
		time.Sleep(modelIdleTimeout + time.Second)
		synctest.Wait()
		if rec.closed.Load() != 1 {
			t.Fatal("model was not released after the queue drained")
		}
	})
}
