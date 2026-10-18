package codec

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mewkiz/flac"
)

type monoReader struct {
	ctx    context.Context
	format pcmFormat
	next   func(context.Context) ([][]int32, error)
	block  [][]int32
	index  int
}

func (r *monoReader) Read(out []float64) (int, error) {
	for i := range out {
		if err := r.ctx.Err(); err != nil {
			return i, err
		}
		if len(r.block) == 0 || r.index == len(r.block[0]) {
			block, err := r.next(r.ctx)
			if err != nil {
				return i, err
			}
			if len(block) != r.format.channels || len(block[0]) == 0 {
				return i, errors.New("invalid audio channel count or empty frame")
			}
			r.block, r.index = block, 0
		}
		value := 0.0
		for _, channel := range r.block {
			if r.index >= len(channel) {
				return i, errors.New("audio channel lengths differ")
			}
			value += float64(channel[r.index])
		}
		out[i] = value / (float64(r.format.channels) * math.Ldexp(1, r.format.bits-1))
		r.index++
	}
	return len(out), nil
}

type flacReader struct {
	stream   *flac.Stream
	checksum hash.Hash
	count    uint64
}

func (r *flacReader) readBlock(ctx context.Context) ([][]int32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := r.stream.ParseNext()
	if err == io.EOF {
		info := r.stream.Info
		if info.NSamples != 0 && info.NSamples != r.count {
			return nil, io.ErrUnexpectedEOF
		}
		if info.MD5sum != [16]byte{} && !bytes.Equal(info.MD5sum[:], r.checksum.Sum(nil)) {
			return nil, errors.New("FLAC audio checksum mismatch")
		}
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	info := r.stream.Info
	if f.Channels.Count() != int(info.NChannels) || (f.SampleRate != 0 && f.SampleRate != info.SampleRate) || f.BitsPerSample != info.BitsPerSample {
		return nil, errors.New("FLAC frame format changed")
	}
	r.count += uint64(f.BlockSize)
	if info.NSamples != 0 && r.count > info.NSamples {
		return nil, errors.New("FLAC sample count exceeds metadata")
	}
	f.Hash(r.checksum)
	channels := make([][]int32, len(f.Subframes))
	for i, subframe := range f.Subframes {
		channels[i] = subframe.Samples
	}
	return channels, nil
}

type contextReader struct {
	context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// ReadMono decodes existing recordings and supplies 16 kHz mono samples to ASR.
// Files are streamed through conversion; only the final ASR input is retained.
func ReadMono(ctx context.Context, path string) ([]float32, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r := &monoReader{ctx: ctx}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		wav, err := openWAV(path)
		if err != nil {
			return nil, 0, err
		}
		defer wav.Close()
		r.format, r.next = wav.pcmFormat, wav.readBlock
	case ".flac":
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer file.Close()
		stream, err := flac.New(contextReader{ctx, file})
		if err != nil {
			return nil, 0, err
		}
		info := stream.Info
		// 32-bit FLAC side-channel arithmetic is not supported by this decoder.
		if info.BitsPerSample > 24 {
			return nil, 0, errors.New("FLAC decoding supports up to 24-bit PCM")
		}
		decoder := &flacReader{stream: stream, checksum: md5.New()}
		r.format = pcmFormat{int(info.SampleRate), int(info.NChannels), int(info.BitsPerSample)}
		r.next = decoder.readBlock
	default:
		return nil, 0, fmt.Errorf("unsupported recording format: %s", filepath.Ext(path))
	}
	samples, err := resample(ctx, r, r.format.rate)
	if err != nil {
		return nil, 0, err
	}
	return samples, SampleRate, nil
}
