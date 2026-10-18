package assets

import (
	"context"
	"net/http"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

func TestSystemModelUsesLanguageKeysWithoutSherpaDownloads(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	if m.selectedModel() != protocol.DefaultASRModel {
		t.Fatal("manager ignored the platform default")
	}
	m.client.Transport = voiceprintTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("system speech models must never use sherpa downloads")
		return nil, nil
	})
	for _, language := range []string{"auto", "zh", "en"} {
		if got := m.ASRModelPath(protocol.ASRSpeechAnalyzer, language); got != speechanalyzer.ModelKey(language) {
			t.Fatal("system model path lost its language", got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.EnsureASRModel(ctx, protocol.ASRSpeechAnalyzer, "en"); err == nil {
		t.Fatal("canceled or unsupported preparation was accepted")
	}
	if m.selectedModel() != protocol.DefaultASRModel {
		t.Fatal("preparation changed the selected model")
	}
	if speechanalyzer.Available() {
		m.SelectASRModel(protocol.ASRSpeechAnalyzer, "ja")
		if m.ASRPath() != speechanalyzer.ModelKey("ja") {
			t.Fatal("selected system language was not retained")
		}
	}
}
