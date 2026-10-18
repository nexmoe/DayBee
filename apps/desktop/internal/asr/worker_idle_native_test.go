package asr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// Opt-in measurement uses cached models and a public WAV, never a microphone
// or a user's recording. Footprint includes native ONNX allocations, not just
// the Go heap. Ordinary tests do not load a model or wait for the real timeout.
func TestNativeIdleModelRelease(t *testing.T) {
	model, wav := os.Getenv("DAYBEE_TEST_IDLE_MODEL_DIR"), os.Getenv("DAYBEE_TEST_IDLE_WAV")
	if model == "" || wav == "" || runtime.GOOS != "darwin" {
		t.Skip("Set DAYBEE_TEST_IDLE_MODEL_DIR and DAYBEE_TEST_IDLE_WAV on macOS for native memory measurement")
	}
	samples, rate, err := (Sherpa{}).ReadWave(wav)
	if err != nil {
		t.Fatal(err)
	}
	footprint := func() int64 {
		t.Helper()
		raw, err := exec.Command("/usr/bin/vmmap", "-summary", strconv.Itoa(os.Getpid())).Output()
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`Physical footprint:\s+([0-9.]+)([KMG])`).FindSubmatch(raw)
		if match == nil {
			t.Fatal("vmmap did not report the physical footprint")
		}
		value, _ := strconv.ParseFloat(string(match[1]), 64)
		multiplier := map[string]float64{"K": 1024, "M": 1024 * 1024, "G": 1024 * 1024 * 1024}
		return int64(value * multiplier[string(match[2])])
	}
	var report struct {
		Model            string  `json:"model"`
		BeforeBytes      int64   `json:"beforeBytes"`
		WarmBytes        int64   `json:"warmBytes"`
		IdleBytes        int64   `json:"idleBytes"`
		ReloadBytes      int64   `json:"reloadBytes"`
		FirstJobSeconds  float64 `json:"firstJobSeconds"`
		ReloadJobSeconds float64 `json:"reloadJobSeconds"`
	}
	report.Model, report.BeforeBytes = model, footprint()
	released := make(chan struct{}, 2)
	w := NewModelWorker(context.Background(), func() string { return model }, func(path string) (Recognizer, error) {
		r, err := (Sherpa{}).NewRecognizer(path)
		if err != nil {
			return nil, err
		}
		return &releaseObservedRecognizer{Recognizer: r, released: released}, nil
	})
	defer w.Close()
	decode := func() (Result, time.Duration) {
		t.Helper()
		start := time.Now()
		done := make(chan Result, 1)
		errors := make(chan error, 1)
		if !w.SubmitLive(Job{Context: context.Background(), ModelPath: model,
			Prepare:  func() ([]float32, int, error) { return samples, rate, nil },
			Complete: func(result Result, err error) { done <- result; errors <- err },
		}) {
			t.Fatal("native speech was rejected")
		}
		select {
		case result := <-done:
			if err := <-errors; err != nil || result.Text == "" {
				t.Fatal("native speech failed", err)
			}
			return result, time.Since(start)
		case <-time.After(time.Minute):
			t.Fatal("native speech timed out")
			return Result{}, 0
		}
	}
	first, duration := decode()
	report.FirstJobSeconds, report.WarmBytes = duration.Seconds(), footprint()
	t.Logf("warm native model: %.1f MiB; waiting %s for idle release", float64(report.WarmBytes)/(1<<20), modelIdleTimeout)
	select {
	case <-released:
	case <-time.After(modelIdleTimeout + 10*time.Second):
		t.Fatal("idle model was not released")
	}
	report.IdleBytes = footprint()
	second, duration := decode()
	report.ReloadJobSeconds, report.ReloadBytes = duration.Seconds(), footprint()
	if first != second {
		t.Fatal("reloading changed native recognition output")
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(raw))
	if path := os.Getenv("DAYBEE_TEST_IDLE_REPORT"); path != "" {
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

type releaseObservedRecognizer struct {
	Recognizer
	released chan<- struct{}
}

func (r *releaseObservedRecognizer) Close() { r.Recognizer.Close(); r.released <- struct{}{} }
