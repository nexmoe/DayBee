package recorder

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/config"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func (e *Engine) CancelBatch() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.batchCancel != nil {
		e.batchCancel()
	}
}

func (e *Engine) Transcribe(directory string) error {
	e.op.Lock()
	if e.closed {
		e.op.Unlock()
		return context.Canceled
	}
	if !e.batch.CompareAndSwap(false, true) {
		e.op.Unlock()
		return errors.New("A transcription is already running.")
	}
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	e.mu.Lock()
	e.batchCancel = cancel
	done := make(chan struct{})
	e.batchDone = done
	e.mu.Unlock()
	model := e.Settings().Recording.ASRModel
	e.op.Unlock()
	defer func() {
		e.mu.Lock()
		e.batchCancel = nil
		e.batch.Store(false)
		close(done)
		e.mu.Unlock()
		e.notify()
	}()
	if err := e.assets.EnsureASRModel(ctx, model); err != nil {
		return err
	}
	modelPath := e.assets.ASRModelPath(model)
	files, err := AudioFiles([]string{directory})
	if err != nil {
		return err
	}
	processed := 0
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, active := e.active.Load(path); active {
			continue
		}
		jsonPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
		e.patch(func(s *protocol.RecorderStatusSnapshot) { s.StatusMessage = "Transcribing " + filepath.Base(path) })
		labeled, err := e.transcribeLabeled(ctx, path, jsonPath, model, modelPath)
		if err != nil {
			return err
		}
		if labeled {
			processed++
			e.historyUpdated()
			e.notify()
			continue
		}
		type completion struct {
			result asr.Result
			err    error
		}
		done := make(chan completion, 1)
		job := asr.Job{Context: ctx, ModelPath: modelPath, Prepare: func() ([]float32, int, error) {
			return e.assets.ReadAudio(ctx, path)
		}, Complete: func(result asr.Result, err error) { done <- completion{result, err} }}
		if err := e.worker.SubmitBatch(job); err != nil {
			return err
		}
		// Even cancellation waits for an already-started native decode to finish.
		completed := <-done
		if err := ctx.Err(); err != nil {
			return err
		}
		if completed.err != nil {
			e.fail(completed.err)
			return completed.err
		}
		metadata := batchMetadata(path, jsonPath, completed.result.Text, completed.result.Language, model)
		if err := config.WriteJSON(jsonPath, metadata); err != nil {
			return err
		}
		processed++
		e.historyUpdated()
		e.notify()
	}
	suffix := "s"
	if processed == 1 {
		suffix = ""
	}
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.StatusMessage = fmt.Sprintf("Transcribed %d recording%s.", processed, suffix)
	})
	if e.Settings().Recording.DisableASR {
		e.worker.ReleaseWhenIdle()
	}
	return nil
}
