package recorder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

type blockingPreparation struct {
	*fakeAssets
	beforeEnsure bool
	entered      chan struct{}
	release      chan struct{}
}

func (a *blockingPreparation) Ensure(ctx context.Context, _, _ bool) error {
	if a.beforeEnsure {
		close(a.entered)
		<-a.release
	} else {
		close(a.entered)
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestStopCancelsPreparationBeforeOrDuringDownload(t *testing.T) {
	for _, before := range []bool{false, true} {
		t.Run(map[bool]string{false: "during", true: "before"}[before], func(t *testing.T) {
			settings := protocol.DefaultSettings(t.TempDir())
			settings.Recording.AudioFormat = protocol.FLAC
			settings.Recording.AutoSwitchDevice = false
			assets := &blockingPreparation{fakeAssets: &fakeAssets{}, entered: make(chan struct{}), release: make(chan struct{})}
			capture := &fakeCapture{}
			e := New(settings, capture, assets, &fakeRuntime{}, nil, nil)
			defer e.Close()
			assets.beforeEnsure = before
			started, stopped := make(chan error, 1), make(chan error, 1)
			go func() { started <- e.Start() }()
			select {
			case <-assets.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("preparation never started")
			}
			go func() { stopped <- e.Stop() }()
			deadline := time.Now().Add(3 * time.Second)
			for before && e.halts.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			close(assets.release)
			select {
			case err := <-started:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("preparation was not canceled", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("start blocked shutdown")
			}
			if err := <-stopped; err != nil || e.Status().Recording {
				t.Fatal("stop did not finish", err)
			}
		})
	}
}

func TestBatchReprocessesExistingTranscript(t *testing.T) {
	e, _, _, root := testEngine(t, true, &fakeRuntime{})
	path := filepath.Join(root, "old.wav")
	if err := os.WriteFile(path, []byte("fake runtime reads fixed samples"), 0600); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(root, "old.json")
	if err := os.WriteFile(jsonPath, []byte(`{"text":"previous transcript"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Transcribe(root); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.HistoryChanges():
	default:
		t.Fatal("batch result did not notify the timeline")
	}
	data, _ := os.ReadFile(jsonPath)
	if strings.Contains(string(data), "previous transcript") || !strings.Contains(string(data), "固定测试文本") {
		t.Fatal("explicit reprocessing skipped existing JSON", string(data))
	}
	if e.Status().StatusMessage != "Transcribed 1 recording." {
		t.Fatal("batch completion changed existing status")
	}
}
