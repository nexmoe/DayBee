// Package speechanalyzer bridges Apple's on-device Speech framework without
// making the rest of the application require macOS 26.
package speechanalyzer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const KeyPrefix = "speech-analyzer:"

var ErrUnavailable = errors.New("SpeechAnalyzer requires macOS 26 or later and a compatible Mac.")

type Result struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Ready    bool   `json:"ready"`
	Error    string `json:"error"`
}

func ModelKey(language string) string {
	if strings.TrimSpace(language) == "" {
		language = "auto"
	}
	return KeyPrefix + language
}

func IsModelKey(key string) bool { return strings.HasPrefix(key, KeyPrefix) }

func Language(key string) string { return strings.TrimPrefix(key, KeyPrefix) }

var readiness sync.Map

type readyState struct {
	ready bool
	at    time.Time
}

// Cache the system inventory: snapshots are published frequently and must not
// issue an XPC request on every frame. Successful preparation updates it at once.
func Ready(language string) bool {
	if !Available() {
		return false
	}
	if value, ok := readiness.Load(language); ok {
		state := value.(readyState)
		if time.Since(state.at) < time.Minute {
			return state.ready
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := request(ctx, "status", language, nil, 0)
	ready := err == nil && result.Ready
	readiness.Store(language, readyState{ready, time.Now()})
	return ready
}

func Ensure(ctx context.Context, language string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	result, err := request(ctx, "prepare", language, nil, 0)
	if err == nil && !result.Ready {
		err = errors.New("SpeechAnalyzer system assets are not ready after preparation.")
	}
	if err == nil {
		readiness.Store(language, readyState{result.Ready, time.Now()})
	}
	return err
}

func Decode(ctx context.Context, language string, samples []float32, rate int) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(samples) == 0 {
		return Result{}, nil
	}
	if rate <= 0 {
		return Result{}, errors.New("SpeechAnalyzer requires a positive sample rate.")
	}
	return request(ctx, "decode", language, samples, rate)
}
