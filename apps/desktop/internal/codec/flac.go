package codec

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

const FLACVersion = "1.0.14"

// bufferedFile deliberately does not implement Close: Encoder.Close rewrites
// STREAMINFO, then we flush, sync and close the file before publishing it.
type bufferedFile struct {
	file   *os.File
	buffer *bufio.Writer
}

func (f *bufferedFile) Write(p []byte) (int, error) { return f.buffer.Write(p) }
func (f *bufferedFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.buffer.Flush(); err != nil {
		return 0, err
	}
	return f.file.Seek(offset, whence)
}

// EncodeFLAC streams integer PCM without changing its samples or deleting the
// source. A failed or canceled conversion never publishes a partial FLAC.
func EncodeFLAC(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wav, err := openWAV(source)
	if err != nil {
		return err
	}
	defer wav.Close()
	// The encoder's prediction arithmetic is int32. Recordings use 16 bits;
	// accepting 32-bit PCM here could overflow the predictor.
	if wav.bits > 24 {
		return errors.New("FLAC encoding supports 8, 16 and 24-bit PCM")
	}
	if wav.remaining == 0 {
		return errors.New("cannot encode an empty recording")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("recording already exists or is inaccessible: %s", destination)
	}
	stage, err := os.CreateTemp(filepath.Dir(destination), ".daybee-*.flac")
	if err != nil {
		return err
	}
	defer os.Remove(stage.Name())
	defer stage.Close()
	writer := &bufferedFile{stage, bufio.NewWriterSize(stage, 65536)}
	info := &meta.StreamInfo{BlockSizeMin: 4096, BlockSizeMax: 4096, SampleRate: uint32(wav.rate), NChannels: uint8(wav.channels), BitsPerSample: uint8(wav.bits)}
	encoder, err := flac.NewEncoder(writer, info)
	if err != nil {
		return err
	}
	for {
		channels, err := wav.readBlock(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		n := len(channels[0])
		f := &frame.Frame{Header: frame.Header{BlockSize: uint16(n), SampleRate: uint32(wav.rate), Channels: frame.Channels(wav.channels - 1), BitsPerSample: uint8(wav.bits)}}
		for _, samples := range channels {
			f.Subframes = append(f.Subframes, &frame.Subframe{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: samples, NSamples: n})
		}
		if err := encoder.WriteFrame(f); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := writer.buffer.Flush(); err != nil {
		return err
	}
	// A final frame may have fewer than 16 samples. Upstream includes that tail
	// in its minimum; STREAMINFO still requires both declared sizes to be >=16.
	var sizes [4]byte
	binary.BigEndian.PutUint16(sizes[:2], max(16, info.BlockSizeMin))
	binary.BigEndian.PutUint16(sizes[2:], max(16, info.BlockSizeMax))
	if _, err := stage.WriteAt(sizes[:], 8); err != nil {
		return err
	}
	if err := stage.Sync(); err != nil {
		return err
	}
	if err := stage.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("recording already exists or is inaccessible: %s", destination)
	}
	return os.Rename(stage.Name(), destination)
}
