package asr

import (
	"context"
	"errors"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

type speechRecognizer struct {
	language string
	closed   bool
}

func newSpeechRecognizer(key string) (Recognizer, error) {
	if !speechanalyzer.Available() {
		return nil, speechanalyzer.ErrUnavailable
	}
	return &speechRecognizer{language: speechanalyzer.Language(key)}, nil
}

func (r *speechRecognizer) Decode(samples []float32, rate int) (Result, error) {
	if r.closed {
		return Result{}, errors.New("ASR recognizer is closed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := speechanalyzer.Decode(ctx, r.language, samples, rate)
	return Result{Text: result.Text, Language: result.Language}, err
}

func (r *speechRecognizer) Close() { r.closed = true }
