package assets

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

// Tiny deterministic tar.bz2 fixtures contain model.int8.onnx and, in the
// complete archive, tokens.txt under the pinned FireRed CTC directory.
const completeCTC = "QlpoOTFBWSZTWWNVyWAAAMR7gMmAAIJQA//AAADvb95QCQgwALiwlSABoAAAAGgaUGmjQ9QAAAGhoFVNJMmp6J5R6jZDU8oGNIP7vraUPaARSJSX3z7PVYYvFYqEkCJMXF4M0LnRmuaPuvXRyWkrUEnPnQMkLpPSpgpTaTTTjFwbStk5ZS+mxqobkIYNYhF+x0+OChRaMHHYQJ6WF0LeI2R7K68Sm4kUPCAoK9Y+EhaCgCxK6IzDAkmzBi8EQH4u5IpwoSDGq5LA"
const incompleteCTC = "QlpoOTFBWSZTWXAztPwAAI17gMmAAApAA//AAADvZ95QCAggAHIakHqZNqaAAANBkEoU1MIeoPUemieoPUND1K/doyCB0cIghw9wvI7MLgEFBUjjGa7vM9/Wl85PEeohJgyV6O5uabM0YWl2XwpZpZrVYaPSVMLa1dE30H5BMPhuHJVRwb1Et0hLCem3PIGeKEIA/F3JFOFCQcDO0/A="

// Include unused FP32 weights and documentation, as the official archive does.
const completeWhisperTiny = "QlpoOTFBWSZTWYBRUlAAAWpfgMnAQAP/wSYCEABua97gCIgwAO2zYRUk9EaNANGgMgDEAikpiDRoADTQAA0EUlBp6mgAAAAHqH68La8oMRkSABCVVTVGmFOr2OwaAYjmGYVDAiQAwtA4jcGWEZG7WSPdoc4zHdkkK0jAK136isnEaICIMs12IHNRKJhiIwngVDhJCA0zTRKiU08q7LLJhuHsM4/R4j4uhC6QfBeHctHAX0DEIjQPUfo3GdSUceCskJFDAJCkwqthfHuNY4hmAT/NIKxtH3OXh1pSuG0ZDOJVCmwQP4u5IpwoSEAoqSgA"

func fixtureModel(t *testing.T, root string, model asrmodel.Model) string {
	t.Helper()
	directory := filepath.Join(root, "models", model.Directory)
	for _, name := range model.Files {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("cached"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestEveryModelReusesItsOwnLegacyCache(t *testing.T) {
	legacy := t.TempDir()
	m := New(t.TempDir(), []string{legacy}, nil)
	for _, spec := range asrmodel.All() {
		if spec.ID == protocol.ASRSpeechAnalyzer {
			continue
		}
		want := fixtureModel(t, legacy, spec)
		m.SelectASRModel(spec.ID)
		if err := m.Ensure(context.Background(), false, true); err != nil {
			t.Fatal(err)
		}
		if m.ASRPath() != want {
			t.Fatal("wrong model cache", m.ASRPath(), want)
		}
	}
	for _, model := range m.Models().Items {
		if model.ID != protocol.ASRSpeechAnalyzer && !model.Ready {
			t.Fatal("downloaded model reported missing", model.ID)
		}
	}
}

func TestDownloadCoalescesAndDoesNotSelectBeforeSuccess(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	data, _ := base64.StdEncoding.DecodeString(completeCTC)
	var requests atomic.Int64
	m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "sherpa-onnx-fire-red-asr2-ctc-zh_en-int8-2026-02-25.tar.bz2") {
			t.Errorf("wrong model URL: %s", r.URL)
		}
		if id := m.Models().DownloadingModel; id == nil || *id != protocol.ASRFireRedCTC {
			t.Error("download not attributed to CTC")
		}
		requests.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	var group sync.WaitGroup
	for range 6 {
		group.Go(func() {
			if err := m.EnsureASRModel(context.Background(), protocol.ASRFireRedCTC); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if requests.Load() != 1 {
		t.Fatal("model downloaded more than once")
	}
	if m.selectedModel() != protocol.DefaultASRModel {
		t.Fatal("download changed active model")
	}
	m.SelectASRModel(protocol.ASRFireRedCTC)
	_, ready := m.Ready()
	if !ready || m.Models().DownloadingModel != nil {
		t.Fatal("download status not cleared or model missing")
	}
}

func TestIncompleteArchivePreservesSelectionAndPreviousFiles(t *testing.T) {
	root := t.TempDir()
	m := New(root, nil, nil)
	selected, _ := asrmodel.Find(protocol.ASRWhisperTiny)
	fixtureModel(t, root, selected)
	m.SelectASRModel(selected.ID)
	ctc, _ := asrmodel.Find(protocol.ASRFireRedCTC)
	old := fixtureModel(t, root, ctc)
	if err := os.Remove(filepath.Join(old, "tokens.txt")); err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(incompleteCTC)
	m.client.Transport = voiceprintTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	if err := m.EnsureASRModel(context.Background(), ctc.ID); err == nil {
		t.Fatal("accepted an incomplete model archive")
	}
	before, _ := os.ReadFile(filepath.Join(old, "model.int8.onnx"))
	if string(before) != "cached" || m.selectedModel() != selected.ID {
		t.Fatal("failed download damaged existing data")
	}
	_, ready := m.Ready()
	if !ready {
		t.Fatal("failed download broke selected model")
	}
	if err := m.EnsureASRModel(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown model accepted")
	}
}

func TestWhisperPreparationNeverFallsBackToLargeCachedModel(t *testing.T) {
	root := t.TempDir()
	qwen, _ := asrmodel.Find(protocol.ASRQwen3)
	fixtureModel(t, root, qwen)
	m := New(root, nil, nil)
	m.SelectASRModel(protocol.ASRWhisperTiny)
	if m.selectedModel() != protocol.ASRWhisperTiny {
		t.Fatal("manager did not choose the lightweight default")
	}
	tiny, _ := asrmodel.Find(protocol.ASRWhisperTiny)
	if m.ASRPath() != filepath.Join(root, "models", tiny.Directory) {
		t.Fatal("a cached large model overrode the lightweight default")
	}
	data, err := base64.StdEncoding.DecodeString(completeWhisperTiny)
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if !strings.HasSuffix(r.URL.Path, tiny.Directory+".tar.bz2") {
			t.Errorf("default initialization downloaded a different model: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	if err := m.Ensure(context.Background(), false, true); err != nil {
		t.Fatal("could not prepare the default Whisper model", err)
	}
	if _, ready := m.Ready(); !ready || requests != 1 || !tiny.Ready(m.ASRPath()) {
		t.Fatal("default Whisper initialization did not download a complete model")
	}
	for _, name := range []string{"tiny-encoder.onnx", "tiny-decoder.onnx"} {
		if _, err := os.Stat(filepath.Join(m.ASRPath(), name)); !os.IsNotExist(err) {
			t.Fatal("unused FP32 weights were retained", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.ASRPath(), "README.md")); err != nil {
		t.Fatal("model documentation was removed while pruning weights", err)
	}
	m.SelectASRModel("unknown")
	if m.selectedModel() != protocol.DefaultASRModel {
		t.Fatal("invalid model did not use the platform fallback")
	}
	m.SelectASRModel(protocol.ASRQwen3)
	if m.ASRPath() != filepath.Join(root, "models", qwen.Directory) {
		t.Fatal("explicit large model selection was not respected")
	}
}
