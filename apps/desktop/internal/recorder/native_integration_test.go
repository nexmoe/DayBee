package recorder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/codec"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type nativeTestAssets struct {
	*fakeAssets
	model, vad string
}

func (a nativeTestAssets) VADPath() string                       { return a.vad }
func (a nativeTestAssets) ASRPath() string                       { return a.model }
func (a nativeTestAssets) ASRModelPath(protocol.ASRModel) string { return a.model }
func (a nativeTestAssets) Transcode(ctx context.Context, source, destination string) error {
	return codec.EncodeFLAC(ctx, source, destination)
}
func (a nativeTestAssets) ReadAudio(ctx context.Context, source string) ([]float32, int, error) {
	return codec.ReadMono(ctx, source)
}

type countedSherpa struct {
	asr.Sherpa
	created int
}

func (r *countedSherpa) NewRecognizer(path string) (asr.Recognizer, error) {
	r.created++
	return r.Sherpa.NewRecognizer(path)
}

// Fixed PCM replaces the microphone only. VAD, ASR, speech-only archive,
// segmentation, finalizers and JSON all use the production implementation.
func TestNativeRecordingPipeline(t *testing.T) {
	model, vad, fixtures := os.Getenv("DAYBEE_TEST_MODEL_DIR"), os.Getenv("DAYBEE_TEST_VAD_PATH"), os.Getenv("DAYBEE_TEST_AUDIO_DIR")
	if model == "" || vad == "" || fixtures == "" {
		t.Skip("Set DAYBEE_TEST_MODEL_DIR, DAYBEE_TEST_VAD_PATH and DAYBEE_TEST_AUDIO_DIR for native recording integration")
	}
	spec, ok := asrmodel.ForDirectory(model)
	if !ok {
		t.Fatal("native test model must be a supported catalog directory", model)
	}
	samples, rate, err := (asr.Sherpa{}).ReadWave(filepath.Join(fixtures, "chinese.wav"))
	if err != nil || rate != audio.SampleRate {
		t.Fatal("Need a fixed 16 kHz Chinese sample", err, rate)
	}
	for _, format := range []protocol.AudioFormat{protocol.WAV, protocol.FLAC} {
		for _, disabled := range []bool{false, true} {
			t.Run(string(format)+"-"+map[bool]string{false: "transcription", true: "pure-recording"}[disabled], func(t *testing.T) {
				root := t.TempDir()
				settings := protocol.DefaultSettings(root)
				settings.Recording.ASRModel = spec.ID
				settings.Recording.AudioFormat = format
				settings.Recording.DisableASR = disabled
				settings.Recording.AutoSwitchDevice = false
				assets := nativeTestAssets{&fakeAssets{}, model, vad}
				runtime := &countedSherpa{}
				e := New(settings, &fakeCapture{}, assets, runtime, nil, nil)
				defer e.Close()
				if err := e.Start(); err != nil {
					t.Fatal(err)
				}
				for offset := 0; offset < len(samples); offset += 512 {
					packet := audio.Packet{Samples: samples[offset:min(offset+512, len(samples))], DeviceID: "default", DeviceLabel: "Fixed PCM"}
					if err := e.call(func() error { return e.consume(packet) }); err != nil {
						t.Fatal(err)
					}
				}
				if err := e.Stop(); err != nil {
					t.Fatal(err)
				}
				if e.Busy() {
					t.Fatal("native recording left work unfinished")
				}
				files, err := AudioFiles([]string{root})
				if err != nil || len(files) != 1 {
					t.Fatal("speech archive missing", files, err)
				}
				if filepath.Ext(files[0]) != "."+string(format) {
					t.Fatal("wrong archive format", files[0])
				}
				speech, rate, err := assets.ReadAudio(context.Background(), files[0])
				if err != nil || rate != audio.SampleRate || len(speech) == 0 {
					t.Fatal("invalid speech-only WAV", err)
				}
				path := strings.TrimSuffix(files[0], filepath.Ext(files[0])) + ".json"
				data, err := os.ReadFile(path)
				if disabled {
					if !os.IsNotExist(err) || runtime.created != 0 || assets.asrRequested {
						t.Fatal("pure recording initialized ASR or wrote JSON")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var metadata struct{ Status, Text, Model string }
				if err := json.Unmarshal(data, &metadata); err != nil {
					t.Fatal(err)
				}
				if metadata.Status != "ok" || strings.TrimSpace(metadata.Text) == "" || metadata.Model != spec.Name || runtime.created != 1 {
					t.Fatal("native transcription failed", string(data))
				}
				t.Logf("Archived %d speech samples and completed native transcript: %s", len(speech), metadata.Text)
				if err := e.Transcribe(root); err != nil {
					t.Fatal("native batch reprocessing failed", err)
				}
				data, err = os.ReadFile(path)
				if err != nil || json.Unmarshal(data, &metadata) != nil || metadata.Status != "ok" || strings.TrimSpace(metadata.Text) == "" || metadata.Model != spec.Name {
					t.Fatal("native batch transcription did not retain the selected model", err)
				}
			})
		}
	}
}
