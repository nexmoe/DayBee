package codec

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/mewkiz/flac"
)

func writePCM(t *testing.T, rate, channels, bits, count int, sample func(int, int) int32) string {
	t.Helper()
	align := channels * bits / 8
	data := make([]byte, 44+count*align)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], uint16(channels))
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(rate*align))
	binary.LittleEndian.PutUint16(data[32:], uint16(align))
	binary.LittleEndian.PutUint16(data[34:], uint16(bits))
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(count*align))
	for i := range count {
		for ch := range channels {
			v := sample(i, ch)
			for b := range bits / 8 {
				data[44+i*align+ch*(bits/8)+b] = byte(v >> uint(b*8))
			}
			if bits == 8 {
				data[44+i*align+ch] = byte(v + 128)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFLACPreservesEverySampleAndShortTail(t *testing.T) {
	t.Setenv("PATH", "") // Conversion cannot silently depend on an external tool.
	for _, bits := range []int{8, 16, 24} {
		for _, count := range []int{1, 15, 16, 4095, 4096, 4097, 8200, 16037} {
			t.Run(fmt.Sprintf("%dbit-%dsamples", bits, count), func(t *testing.T) {
				sample := func(i, ch int) int32 { return int32((i*7919+ch*131+12345)%(1<<bits) - (1 << (bits - 1))) }
				wav := writePCM(t, 16000, 2, bits, count, sample)
				path := filepath.Join(filepath.Dir(wav), "recording.flac")
				if err := EncodeFLAC(context.Background(), wav, path); err != nil {
					t.Fatal(err)
				}
				stream, err := flac.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				if stream.Info.NSamples != uint64(count) || stream.Info.NChannels != 2 || stream.Info.BitsPerSample != uint8(bits) {
					t.Fatal("incorrect STREAMINFO", stream.Info)
				}
				decoder := &flacReader{stream: stream, checksum: md5.New()}
				index := 0
				for {
					block, err := decoder.readBlock(context.Background())
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					for i := range block[0] {
						for ch := range block {
							if block[ch][i] != sample(index+i, ch) {
								t.Fatalf("PCM differs at %d/%d", index+i, ch)
							}
						}
					}
					index += len(block[0])
				}
				if index != count {
					t.Fatal("truncated audio", index)
				}
				if _, err := os.Stat(wav); err != nil {
					t.Fatal("source WAV removed", err)
				}
			})
		}
	}
}

func TestIndependentReferenceAndLegacyFLAC(t *testing.T) {
	samples, rate, err := ReadMono(context.Background(), "testdata/legacy-mono.flac")
	if err != nil || rate != 16000 || len(samples) != 16037 {
		t.Fatal(rate, len(samples), err)
	}
	for i, value := range samples {
		expected := float32((i*7919+12345)%65536-32768) / 32768
		if value != expected {
			t.Fatalf("legacy PCM differs at %d", i)
		}
	}
	for _, file := range []string{"reference-stereo.flac", "reference-24bit.flac"} {
		samples, rate, err := ReadMono(context.Background(), filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(file, err)
		}
		if rate != 16000 || len(samples) != 16000 {
			t.Fatal("duration or rate changed", file, rate, len(samples))
		}
		amplitude := .3
		if file == "reference-24bit.flac" {
			amplitude = .4
		}
		checkTone(t, samples, 440, amplitude)
	}
}

func checkTone(t *testing.T, samples []float32, hz, amplitude float64) {
	t.Helper()
	var power float64
	crossings := 0
	for i, v := range samples {
		power += float64(v) * float64(v)
		if i > 0 && samples[i-1] < 0 && v >= 0 {
			crossings++
		}
	}
	if math.Abs(math.Sqrt(power/float64(len(samples)))-amplitude/math.Sqrt2) > .002 {
		t.Fatal("signal level changed", power)
	}
	if math.Abs(float64(crossings)-hz) > 1 {
		t.Fatal("signal frequency changed", crossings)
	}
}

func TestMonoResamplingAndAntiAliasing(t *testing.T) {
	for _, rate := range []int{8000, 16000, 22050, 44100, 48000, 96000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			wav := writePCM(t, rate, 2, 16, rate, func(i, ch int) int32 {
				return int32(math.Round(math.Sin(2*math.Pi*440*float64(i)/float64(rate)) * (.4 - float64(ch)*.2) * 32767))
			})
			samples, outRate, err := ReadMono(context.Background(), wav)
			if err != nil || outRate != 16000 || len(samples) != 16000 {
				t.Fatal(len(samples), outRate, err)
			}
			checkTone(t, samples, 440, .3)
		})
	}
	wav := writePCM(t, 48000, 1, 16, 48000, func(i, _ int) int32 { return int32(math.Sin(2*math.Pi*13000*float64(i)/48000) * 30000) })
	samples, _, err := ReadMono(context.Background(), wav)
	if err != nil {
		t.Fatal(err)
	}
	var power float64
	for _, v := range samples[100 : len(samples)-100] {
		power += float64(v) * float64(v)
	}
	if math.Sqrt(power/float64(len(samples)-200)) > .001 {
		t.Fatal("out-of-band tone aliases into ASR audio")
	}
}

func TestDecodeRejectsDamagedRecordings(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-mono.flac")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"checksum", "truncated", "crc"} {
		t.Run(name, func(t *testing.T) {
			data := slices.Clone(raw)
			switch name {
			case "checksum":
				data[26] ^= 1
			case "truncated":
				data = data[:len(data)-10]
			case "crc":
				data[len(data)-1] ^= 1
			}
			path := filepath.Join(t.TempDir(), "damaged.flac")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadMono(context.Background(), path); err == nil {
				t.Fatal("damaged FLAC accepted")
			}
		})
	}
	wav := writePCM(t, 16000, 1, 16, 32, func(i, _ int) int32 { return int32(i) })
	if err := os.Truncate(wav, 50); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMono(context.Background(), wav); err == nil {
		t.Fatal("truncated WAV accepted")
	}
}

type cancelAfterChecks struct {
	context.Context
	cancel context.CancelFunc
	checks atomic.Int32
}

func (c *cancelAfterChecks) Err() error {
	if c.checks.Add(-1) <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCancellationAndDestinationProtection(t *testing.T) {
	wav := writePCM(t, 16000, 1, 16, 160000, func(i, _ int) int32 { return int32(i%65536 - 32768) })
	destination := filepath.Join(filepath.Dir(wav), "recording.flac")
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelAfterChecks{Context: base, cancel: cancel}
	ctx.checks.Store(5)
	if err := EncodeFLAC(ctx, wav, destination); !errors.Is(err, context.Canceled) {
		t.Fatal("encoding did not cancel", err)
	}
	entries, err := os.ReadDir(filepath.Dir(wav))
	if err != nil || len(entries) != 1 {
		t.Fatal("partial FLAC left behind", entries, err)
	}
	if _, _, err := ReadMono(ctx, "testdata/legacy-mono.flac"); !errors.Is(err, context.Canceled) {
		t.Fatal("decoding did not cancel", err)
	}
	if err := os.WriteFile(destination, []byte("existing recording"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EncodeFLAC(context.Background(), wav, destination); err == nil {
		t.Fatal("existing destination replaced")
	}
	data, _ := os.ReadFile(destination)
	if string(data) != "existing recording" {
		t.Fatal("existing recording changed")
	}
}
