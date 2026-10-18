package assets

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestModelDownloadFallsBackWhenGitHubIsUnavailable(t *testing.T) {
	t.Setenv("DAYBEE_DOWNLOAD_MIRROR", "global")
	m := New(t.TempDir(), nil, nil)
	data, _ := base64.StdEncoding.DecodeString(completeCTC)
	var hosts []string
	m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		if r.URL.Host == "github.com" {
			return nil, errors.New("GitHub is unreachable")
		}
		if r.URL.Host != "ghfast.top" {
			t.Fatalf("unexpected mirror %s", r.URL)
		}
		if !strings.HasPrefix(r.URL.Path, "/https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/") {
			t.Fatalf("mirror lost the original URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	if err := m.EnsureASRModel(context.Background(), "firered-asr2-ctc"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hosts, []string{"github.com", "ghfast.top"}) {
		t.Fatal("did not promptly fall back to a working mirror", hosts)
	}
}

func TestDownloadRejectsHTMLWithoutReplacingCache(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	path := m.root + ".onnx"
	if err := os.WriteFile(path, []byte("cached"), 0600); err != nil {
		t.Fatal(err)
	}
	m.client.Transport = voiceprintTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader("<html>Download unavailable</html>"))}, nil
	})
	if err := m.downloadOnce(context.Background(), "https://github.com/model.onnx", path, "test"); err == nil {
		t.Fatal("an error page must not be accepted as a model")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "cached" {
		t.Fatal("error page replaced the existing model")
	}
}

type stalledDownloadBody struct{ ctx context.Context }

func (b stalledDownloadBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (stalledDownloadBody) Close() error { return nil }

func TestDownloadFallsBackWhenHeadersOrBodyStall(t *testing.T) {
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				t.Setenv("DAYBEE_DOWNLOAD_MIRROR", "global")
				m := New(t.TempDir(), nil, nil)
				m.idleTimeout = 20 * time.Millisecond
				var requests int
				m.client.Transport = voiceprintTransport(func(r *http.Request) (*http.Response, error) {
					requests++
					if r.URL.Host == "github.com" {
						if phase == "headers" {
							<-r.Context().Done()
							return nil, r.Context().Err()
						}
						return &http.Response{StatusCode: 200, ContentLength: 6, Body: stalledDownloadBody{r.Context()}}, nil
					}
					return &http.Response{StatusCode: 200, ContentLength: 6, Body: io.NopCloser(strings.NewReader("mirror"))}, nil
				})
				path := m.root + ".onnx"
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				start := time.Now()
				if err := m.download(ctx, "https://github.com/model.onnx", path, "test"); err != nil {
					t.Fatal("stalled source did not yield to mirror", err)
				}
				data, _ := os.ReadFile(path)
				if string(data) != "mirror" || requests != 2 {
					t.Fatal("wrong source or number of attempts", string(data), requests)
				}
				if elapsed := time.Since(start); elapsed != m.idleTimeout {
					t.Fatal("fallback did not happen at the idle deadline", elapsed)
				}
			})
		})
	}
}

func TestDownloadCancellationDoesNotTryMirrors(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	var requests int
	m.client.Transport = voiceprintTransport(func(*http.Request) (*http.Response, error) {
		requests++
		cancel()
		return nil, context.Canceled
	})
	err := m.download(ctx, "https://github.com/model.onnx", m.root+".onnx", "test")
	if !errors.Is(err, context.Canceled) || requests != 1 {
		t.Fatal("cancellation retried the download", err, requests)
	}
}

func TestDownloadSourceOrdering(t *testing.T) {
	const original = "https://github.com/model.tar.bz2"
	global := []string{original, "https://ghfast.top/" + original, "https://gh-proxy.com/" + original}
	china := []string{global[1], global[2], global[0]}
	for _, test := range []struct {
		preferChina bool
		want        []string
	}{{false, global}, {true, china}} {
		if got := githubDownloadURLs(original, test.preferChina); !reflect.DeepEqual(got, test.want) {
			t.Fatal("wrong source ordering", got)
		}
	}
	const external = "https://example.com/github.com/model.onnx"
	if got := githubDownloadURLs(external, true); !reflect.DeepEqual(got, []string{external}) {
		t.Fatal("proxied a non-GitHub URL", got)
	}
	for _, mode := range []string{"cn", "global"} {
		t.Setenv("DAYBEE_DOWNLOAD_MIRROR", mode)
		if preferChinaMirrors() != (mode == "cn") {
			t.Fatal("ignored explicit mirror preference", mode)
		}
	}
}
