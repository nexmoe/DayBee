// Package codec implements local WAV/FLAC conversion without subprocesses or CGO.
package codec

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const SampleRate = 16000

type pcmFormat struct {
	rate, channels, bits int
}

type wavReader struct {
	*os.File
	pcmFormat
	remaining int64
}

func openWAV(path string) (_ *wavReader, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var header [12]byte
	if _, err = io.ReadFull(f, header[:]); err != nil {
		return nil, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return nil, errors.New("unsupported WAV container")
	}
	var format pcmFormat
	for {
		var chunk [8]byte
		if _, err = io.ReadFull(f, chunk[:]); err != nil {
			return nil, fmt.Errorf("WAV chunk: %w", err)
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		offset, seekErr := f.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return nil, seekErr
		}
		if size > stat.Size()-offset {
			return nil, io.ErrUnexpectedEOF
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 || size > 65536 {
				return nil, errors.New("invalid WAV format chunk")
			}
			data := make([]byte, size)
			if _, err = io.ReadFull(f, data); err != nil {
				return nil, err
			}
			encoding := binary.LittleEndian.Uint16(data)
			if encoding == 0xfffe && len(data) >= 40 {
				// WAVE_FORMAT_EXTENSIBLE with integer PCM and no reduced precision.
				guid := [16]byte{1, 0, 0, 0, 0, 0, 16, 0, 128, 0, 0, 170, 0, 56, 155, 113}
				if string(data[24:40]) == string(guid[:]) && binary.LittleEndian.Uint16(data[18:]) == binary.LittleEndian.Uint16(data[14:]) {
					encoding = 1
				}
			}
			format = pcmFormat{int(binary.LittleEndian.Uint32(data[4:])), int(binary.LittleEndian.Uint16(data[2:])), int(binary.LittleEndian.Uint16(data[14:]))}
			if encoding != 1 || format.channels < 1 || format.channels > 8 || format.rate < 1 || format.rate > 655350 || (format.bits != 8 && format.bits != 16 && format.bits != 24 && format.bits != 32) {
				return nil, errors.New("unsupported WAV sample format; expected integer PCM")
			}
			if int(binary.LittleEndian.Uint16(data[12:])) != format.channels*(format.bits/8) {
				return nil, errors.New("invalid WAV block alignment")
			}
		case "data":
			if format.channels == 0 {
				return nil, errors.New("WAV data precedes format")
			}
			if size%int64(format.channels*(format.bits/8)) != 0 {
				return nil, errors.New("incomplete WAV sample")
			}
			return &wavReader{f, format, size}, nil
		default:
			if _, err = f.Seek(size, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
		if size%2 != 0 {
			if _, err = f.Seek(1, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}
}

func (w *wavReader) readBlock(ctx context.Context) ([][]int32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.remaining == 0 {
		return nil, io.EOF
	}
	align := w.channels * (w.bits / 8)
	n := min(w.remaining/int64(align), 4096)
	data := make([]byte, int(n)*align)
	if _, err := io.ReadFull(w.File, data); err != nil {
		return nil, err
	}
	w.remaining -= int64(len(data))
	channels := make([][]int32, w.channels)
	for ch := range channels {
		channels[ch] = make([]int32, int(n))
		for i := range channels[ch] {
			p := data[i*align+ch*(w.bits/8):]
			switch w.bits {
			case 8:
				channels[ch][i] = int32(p[0]) - 128
			case 16:
				channels[ch][i] = int32(int16(binary.LittleEndian.Uint16(p)))
			case 24:
				channels[ch][i] = int32(uint32(p[0])|uint32(p[1])<<8|uint32(p[2])<<16) << 8 >> 8
			case 32:
				channels[ch][i] = int32(binary.LittleEndian.Uint32(p))
			}
		}
	}
	return channels, nil
}
