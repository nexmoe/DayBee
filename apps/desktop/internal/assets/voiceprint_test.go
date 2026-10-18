package assets

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

type voiceprintTransport func(*http.Request) (*http.Response, error)

func (f voiceprintTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVoiceprintDownloadCoalescesIndependently(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	m.asrGate <- struct{}{}
	m.vadGate <- struct{}{}
	var downloads atomic.Int32
	m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/speaker-recongition-models/"+voiceprint.ModelName) {
			t.Errorf("unexpected model URL: %s", r.URL)
		}
		downloads.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("model")), ContentLength: 5}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var tasks sync.WaitGroup
	for range 8 {
		tasks.Go(func() {
			if err := m.EnsureVoiceprint(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	tasks.Wait()
	if downloads.Load() != 1 || !usableFile(m.VoiceprintPath()) {
		t.Fatal("concurrent enrollment downloaded the model more than once")
	}
}

func TestVoiceprintReusesLegacyCacheAndCancelsGate(t *testing.T) {
	legacy := t.TempDir()
	path := filepath.Join(legacy, "models", voiceprint.ModelName)
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, []byte("cached"), 0600)
	m := New(t.TempDir(), []string{legacy}, nil)
	if m.VoiceprintPath() != path {
		t.Fatal("legacy model cache was ignored")
	}
	if err := m.EnsureVoiceprint(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.voiceGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.EnsureVoiceprint(ctx); err != context.Canceled {
		t.Fatal("canceled enrollment waited for another model download", err)
	}
}

func TestVoiceprintModelSwitchDoesNotReuseCAMPlusCache(t *testing.T) {
	t.Setenv("DAYBEE_DOWNLOAD_MIRROR", "global")
	m := New(t.TempDir(), []string{t.TempDir()}, nil)
	var oldPaths []string
	for _, cacheRoot := range m.roots {
		path := filepath.Join(cacheRoot, "3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx")
		if err := os.MkdirAll(cacheRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("old model"), 0600); err != nil {
			t.Fatal(err)
		}
		oldPaths = append(oldPaths, path)
	}
	const model = "3dspeaker_speech_eres2netv2_sv_zh-cn_16k-common.onnx"
	wantPath := filepath.Join(m.root, model)
	if got := m.VoiceprintPath(); got != wantPath {
		t.Fatalf("selected %s, want %s", got, wantPath)
	}
	var downloads atomic.Int32
	m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/k2-fsa/sherpa-onnx/releases/download/speaker-recongition-models/"+model {
			t.Errorf("unexpected model URL: %s", r.URL)
		}
		downloads.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("new model")), ContentLength: 9}, nil
	})
	if err := m.EnsureVoiceprint(context.Background()); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(wantPath); err != nil || string(raw) != "new model" || downloads.Load() != 1 {
		t.Fatal("new model was not downloaded to its own cache", err)
	}
	for _, path := range oldPaths {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "old model" {
			t.Fatal("model switch modified the previous model cache", err)
		}
	}
}
