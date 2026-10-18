package audio

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWAVIsClosedAndClipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "speech.wav")
	writer, err := NewWavWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Append([]float32{-2, -.5, 0, .5, 2}); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if len(raw) != 54 || binary.LittleEndian.Uint32(raw[40:]) != 10 || binary.LittleEndian.Uint32(raw[24:]) != 16000 {
		t.Fatal("invalid WAV header")
	}
	if int16(binary.LittleEndian.Uint16(raw[44:])) != -32768 || int16(binary.LittleEndian.Uint16(raw[52:])) != 32767 {
		t.Fatal("invalid PCM clipping")
	}
	if writer.Append([]float32{1}) == nil {
		t.Fatal("accepted audio after close")
	}
}
func TestSpectrumUsesFrequencyBinsAndSmoothing(t *testing.T) {
	s := NewSpectrum()
	samples := make([]float32, 256)
	for i := range samples {
		samples[i] = float32(.01 * math.Sin(2*math.Pi*16*float64(i)/256))
	}
	s.Push(samples)
	first := s.Frame()
	next := s.Frame()
	if len(first) != 128 || first[16] <= first[8] || next[16] <= first[16] {
		t.Fatal("not a smoothed frequency spectrum", first[16], next[16])
	}
	// Independent Blackman center-bin magnitude: 0.01/2 * 0.42,
	// then 0.2 smoothing on the first observation.
	want := int(math.Floor(255 * (20*math.Log10(.01/2*.42*.2) + 100) / 70))
	if math.Abs(float64(first[16]-want)) > 1 {
		t.Fatalf("normalization differs: got %d want %d", first[16], want)
	}
	for _, value := range next {
		if value < 0 || value > 255 {
			t.Fatal("invalid spectrum byte")
		}
	}
	silence := NewSpectrum().Frame()
	for _, value := range silence {
		if value != 0 {
			t.Fatal("silence is not zero")
		}
	}
}
func TestDeviceSwitchHysteresis(t *testing.T) {
	p := SwitchPolicy{}
	now := time.Now()
	if p.Consider(now, "usb", .01, .001, 2) {
		t.Fatal("switched before confirmations")
	}
	if !p.Consider(now.Add(3*time.Second), "usb", .01, .001, 2) {
		t.Fatal("did not switch")
	}
	if p.Consider(now.Add(6*time.Second), "other", .01, .001, 1) {
		t.Fatal("cooldown ignored")
	}
	if p.Consider(now.Add(15*time.Second), "other", .005, 0, 1) {
		t.Fatal("noise threshold ignored")
	}
}

func TestSpectrumMatchesChromiumAnalyser(t *testing.T) {
	// Recorded from an actual Chromium OfflineAudioContext at 16 kHz, with
	// fftSize=256 and smoothingTimeConstant=0.8 after a 1 kHz, 0.01 tone.
	data, err := os.ReadFile("testdata/chromium-analyser.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Spectrum []int }
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 1024)
	for i := range samples {
		samples[i] = float32(.01 * math.Sin(2*math.Pi*16*float64(i)/256))
	}
	spectrum := NewSpectrum()
	spectrum.Push(samples)
	frame := spectrum.Frame()
	if len(fixture.Spectrum) != len(frame) {
		t.Fatal("Invalid browser fixture")
	}
	for i, value := range frame {
		if math.Abs(float64(value-fixture.Spectrum[i])) > 1 {
			t.Fatalf("bin %d: Go=%d Chromium=%d", i, value, fixture.Spectrum[i])
		}
	}
}
