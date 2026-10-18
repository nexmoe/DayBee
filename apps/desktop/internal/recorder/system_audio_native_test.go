package recorder

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/audio"
	"github.com/nexmoe/DayBee/apps/desktop/internal/codec"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// Uses previously captured source WAVs. VAD, ASR, FLAC, metadata and history are
// production code; no microphone or system recording happens in this test.
func TestNativeDualSourceTranscription(t *testing.T) {
	model, vad := os.Getenv("DAYBEE_TEST_MODEL_DIR"), os.Getenv("DAYBEE_TEST_VAD_PATH")
	mic, system := os.Getenv("DAYBEE_TEST_MIC_CAPTURE_WAV"), os.Getenv("DAYBEE_TEST_SYSTEM_CAPTURE_WAV")
	micExpected, systemExpected := os.Getenv("DAYBEE_TEST_MIC_EXPECTED"), os.Getenv("DAYBEE_TEST_SYSTEM_EXPECTED")
	if model == "" || vad == "" || mic == "" || system == "" || micExpected == "" || systemExpected == "" {
		t.Skip("Provide model, VAD, separate source captures and expected phrases for native dual source acceptance")
	}
	inputs := map[protocol.AudioSource][]float32{}
	for source, path := range map[protocol.AudioSource]string{protocol.SourceMicrophone: mic, protocol.SourceSystem: system} {
		samples, rate, err := codec.ReadMono(context.Background(), path)
		if err != nil || rate != audio.SampleRate {
			t.Fatal("source capture must be 16 kHz", source, rate, err)
		}
		inputs[source] = samples
	}
	root := t.TempDir()
	s := protocol.DefaultSettings(root)
	s.Recording.RecordSystemAudio = true
	s.Recording.AutoSwitchDevice = false
	e := New(s, &fakeCapture{}, nativeTestAssets{&fakeAssets{}, model, vad}, asr.Sherpa{}, nil, nil)
	defer e.Close()
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < max(len(inputs[protocol.SourceMicrophone]), len(inputs[protocol.SourceSystem])); offset += audio.WindowSize {
		for _, source := range []protocol.AudioSource{protocol.SourceMicrophone, protocol.SourceSystem} {
			samples := inputs[source]
			if offset >= len(samples) {
				continue
			}
			packet := audio.Packet{Source: source, DeviceLabel: string(source), Samples: samples[offset:min(offset+audio.WindowSize, len(samples))]}
			if err := e.call(func() error { return e.consume(packet) }); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	items := History([]string{root})
	if len(items) != 2 {
		t.Fatalf("expected two native transcript archives: %+v", items)
	}
	for _, item := range items {
		want, excluded := micExpected, systemExpected
		if item.AudioSource == protocol.SourceSystem {
			want, excluded = excluded, want
		}
		if !strings.HasSuffix(item.AudioPath, ".flac") || item.Status != "ok" || len(item.TranscriptChunks) == 0 ||
			!strings.Contains(item.TextPreview, want) || strings.Contains(item.TextPreview, excluded) {
			t.Fatalf("source ASR or FLAC archive is mixed/incorrect: %+v", item)
		}
		t.Logf("%s: %s", item.AudioSource, item.TextPreview)
	}
}
