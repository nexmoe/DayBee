package asr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type Job struct {
	Context context.Context
	// ModelPath pins queued work to the model selected when it was captured.
	// An empty path uses the worker's current model for compatibility.
	ModelPath string
	Prepare   func() ([]float32, int, error)
	Complete  func(Result, error)
	// Spans, when set, decodes every slice on this worker while it holds the
	// recognizer. The job is still queued once. Complete runs after Spans
	// returns, including when the worker discards the job. Prepare is unused.
	Spans func(decode func([]float32, int) (Result, error)) error
}

// Worker owns exactly one recognizer. Neither VAD nor file writing waits for
// inference. Live jobs take priority between offline files, without duplicating
// a multi-gigabyte model in memory.
type Worker struct {
	ctx       context.Context
	cancel    context.CancelFunc
	live      chan Job
	batch     chan Job
	release   chan string
	done      chan struct{}
	factory   func(string) (Recognizer, error)
	key       func() string
	loadedKey string
	pending   atomic.Int64
	mu        sync.Mutex
	closed    bool
}

func NewWorker(parent context.Context, factory func() (Recognizer, error)) *Worker {
	return NewKeyedWorker(parent, nil, factory)
}

// NewKeyedWorker discards the previous recognizer before decoding with a new
// model. The key must support concurrent reads; only the inference goroutine
// calls the factory and owns the recognizer.
func NewKeyedWorker(parent context.Context, key func() string, factory func() (Recognizer, error)) *Worker {
	return NewModelWorker(parent, key, func(string) (Recognizer, error) { return factory() })
}

// NewModelWorker loads the exact model pinned to each job. Model changes happen
// between jobs on the inference goroutine, without blocking audio capture.
func NewModelWorker(parent context.Context, key func() string, factory func(string) (Recognizer, error)) *Worker {
	ctx, cancel := context.WithCancel(parent)
	w := &Worker{ctx: ctx, cancel: cancel, live: make(chan Job, 6), batch: make(chan Job, 1),
		release: make(chan string, 1), done: make(chan struct{}), factory: factory, key: key}
	go w.run()
	return w
}

// Keep a warm model across ordinary pauses, but do not retain its weights and
// native inference arena throughout hours of silence or background recording.
const modelIdleTimeout = 2 * time.Minute

func (w *Worker) SubmitLive(job Job) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.ctx.Err() != nil {
		return false
	}
	w.pending.Add(1)
	select {
	case w.live <- job:
		return true
	default:
		w.pending.Add(-1)
		return false
	}
}

const batchRetry = 10 * time.Millisecond

func (w *Worker) SubmitBatch(job Job) error {
	// Go 1.23+ timers need no drain before Reset; one timer serves every retry.
	timer := time.NewTimer(batchRetry)
	defer timer.Stop()
	for {
		w.mu.Lock()
		if w.closed || w.ctx.Err() != nil {
			w.mu.Unlock()
			return context.Canceled
		}
		w.pending.Add(1)
		select {
		case w.batch <- job:
			w.mu.Unlock()
			return nil
		default:
			w.pending.Add(-1)
			w.mu.Unlock()
		}
		timer.Reset(batchRetry)
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-job.Context.Done():
			return job.Context.Err()
		case <-timer.C:
		}
	}
}

func (w *Worker) Busy() bool { return w.pending.Load() != 0 }
func (w *Worker) ReleaseWhenIdle() {
	key := ""
	if w.key != nil {
		key = w.key()
	}
	select {
	case w.release <- key:
	default:
	}
}
func (w *Worker) Close() { w.mu.Lock(); w.closed = true; w.cancel(); w.mu.Unlock(); <-w.done }

func (w *Worker) complete(job Job, result Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ASR completion failed", "panic", r)
		}
	}()
	defer w.pending.Add(-1)
	if job.Complete != nil {
		job.Complete(result, err)
	}
}

func (w *Worker) run() {
	defer close(w.done)
	var recognizer Recognizer
	var idleTimer *time.Timer
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
		if recognizer != nil {
			recognizer.Close()
		}
	}()
	release := false
	releaseKey := ""
	for {
		if release && !w.Busy() {
			// A delayed request for the old model must not unload a model
			// that was selected and loaded after the request was queued.
			if recognizer != nil && (w.key == nil || w.loadedKey == releaseKey) {
				recognizer.Close()
				recognizer = nil
			}
			release = false
		}
		var idle <-chan time.Time
		if recognizer != nil && !w.Busy() {
			if idleTimer == nil {
				idleTimer = time.NewTimer(modelIdleTimeout)
			} else {
				idleTimer.Reset(modelIdleTimeout)
			}
			idle = idleTimer.C
		} else if idleTimer != nil {
			idleTimer.Stop()
		}
		var job Job
		select {
		case <-w.ctx.Done():
			w.discard()
			return
		case job = <-w.live:
		default:
			select {
			case <-w.ctx.Done():
				w.discard()
				return
			case <-idle:
				// A submit racing the timeout gets the existing warm model.
				if !w.Busy() {
					recognizer.Close()
					recognizer = nil
				}
				continue
			case releaseKey = <-w.release:
				release = true
				continue
			case job = <-w.live:
			case job = <-w.batch:
				// Both queues may become ready after the first nonblocking
				// check. A select chooses randomly, so check live work again
				// before beginning the next offline file.
				w.drainLive(&recognizer)
			}
		}
		result, err := w.execute(job, &recognizer)
		w.complete(job, result, err)
	}
}

func (w *Worker) drainLive(recognizer *Recognizer) {
	for {
		select {
		case job := <-w.live:
			result, err := w.execute(job, recognizer)
			w.complete(job, result, err)
		default:
			return
		}
	}
}

func (w *Worker) execute(job Job, recognizer *Recognizer) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("ASR failed: %v", recovered)
		}
	}()
	if err = w.loadRecognizer(job, recognizer); err != nil {
		return result, err
	}
	if job.Spans != nil {
		return result, job.Spans(w.spanDecode(job, recognizer))
	}
	samples, rate, err := job.Prepare()
	if err != nil {
		return result, err
	}
	if err := job.Context.Err(); err != nil {
		return result, err
	}
	return (*recognizer).Decode(samples, rate)
}

func (w *Worker) loadRecognizer(job Job, recognizer *Recognizer) error {
	if err := job.Context.Err(); err != nil {
		return err
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	key := job.ModelPath
	if key == "" && w.key != nil {
		key = w.key()
	}
	if key != w.loadedKey {
		if *recognizer != nil {
			(*recognizer).Close()
			*recognizer = nil
		}
		w.loadedKey = key
	}
	if *recognizer == nil {
		created, err := w.factory(key)
		if err != nil {
			return err
		}
		*recognizer = created
	}
	return nil
}

func (w *Worker) spanDecode(job Job, recognizer *Recognizer) func([]float32, int) (Result, error) {
	return func(samples []float32, rate int) (Result, error) {
		if err := canceled(job.Context, w.ctx); err != nil {
			return Result{}, err
		}
		result, err := (*recognizer).Decode(samples, rate)
		// Decode may finish after the caller or the worker has already stopped.
		// Drop that result so a late span cannot be stored.
		if ctxErr := canceled(job.Context, w.ctx); ctxErr != nil {
			return Result{}, ctxErr
		}
		return result, err
	}
}

func canceled(job, worker context.Context) error {
	if job != nil {
		if err := job.Err(); err != nil {
			return err
		}
	}
	if worker != nil {
		if err := worker.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) discard() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	for {
		select {
		case job := <-w.live:
			w.complete(job, Result{}, context.Canceled)
		case job := <-w.batch:
			w.complete(job, Result{}, context.Canceled)
		default:
			return
		}
	}
}
