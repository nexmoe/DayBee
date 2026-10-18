package assets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func TestCachesAreReusedWithoutMovingFiles(t *testing.T) {
	root, old := t.TempDir(), t.TempDir()
	dir := filepath.Join(old, "models", asrmodel.QwenDirectory)
	if err := os.MkdirAll(filepath.Join(dir, "tokenizer"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"conv_frontend.onnx", "encoder.int8.onnx", "decoder.int8.onnx", "tokenizer/vocab.json", "tokenizer/merges.txt", "tokenizer/tokenizer_config.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("cached"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(old, "models/silero_vad.onnx"), []byte("cached"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(root, []string{old}, nil)
	m.SelectASRModel(protocol.ASRQwen3)
	if err := m.Ensure(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	if m.ASRPath() != dir || m.VADPath() != filepath.Join(old, "models/silero_vad.onnx") {
		t.Fatal("cache was not reused")
	}
	if _, err := os.Stat(filepath.Join(root, "models", asrmodel.QwenDirectory)); !os.IsNotExist(err) {
		t.Fatal("legacy cache was moved")
	}
}

func TestVADPreparationDoesNotWaitForASRAndGateIsCancelable(t *testing.T) {
	root := t.TempDir()
	m := New(root, nil, nil)
	if err := os.MkdirAll(filepath.Join(root, "models"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.VADPath(), []byte("cached"), 0600); err != nil {
		t.Fatal(err)
	}
	m.asrGate <- struct{}{}
	defer func() { <-m.asrGate }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := m.Ensure(ctx, true, false); err != nil {
		t.Fatal("VAD waited for ASR", err)
	}
	if err := m.Ensure(ctx, false, true); err != context.DeadlineExceeded {
		t.Fatal("gate wait ignored cancellation", err)
	}
}
func TestInterruptedDownloadDoesNotReplaceCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(t.TempDir(), nil, nil)
	if err := m.downloadOnce(context.Background(), server.URL, path, "test"); err == nil {
		t.Fatal("accepted truncated model")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Fatal("download damaged existing cache")
	}
}
func TestVADOnlyDoesNotDownloadASR(t *testing.T) {
	root := t.TempDir()
	m := New(root, nil, nil)
	m.SelectASRModel(protocol.ASRWhisperTiny)
	if err := os.MkdirAll(filepath.Join(root, "models"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.VADPath(), []byte("cached VAD"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	vad, asr := m.Ready()
	if !vad || asr {
		t.Fatal("VAD/ASR readiness not independent")
	}
}
