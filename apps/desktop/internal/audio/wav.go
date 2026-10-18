package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

type WavWriter struct {
	file   *os.File
	bytes  uint32
	closed bool
}

func NewWavWriter(path string) (*WavWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	w := &WavWriter{file: f}
	if _, err := f.Write(w.header()); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

func (w *WavWriter) Append(samples []float32) error {
	if w.closed {
		return errors.New("recording WAV is closed")
	}
	if uint64(w.bytes)+uint64(len(samples))*2 > math.MaxUint32-36 {
		return errors.New("recording exceeds WAV size limit")
	}
	pcm := make([]byte, len(samples)*2)
	for i, sample := range samples {
		value := math.Max(-1, math.Min(1, float64(sample)))
		scale := 32767.0
		if value < 0 {
			scale = 32768
		}
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(value*scale)))
	}
	n, err := w.file.Write(pcm)
	w.bytes += uint32(n)
	return err
}

func (w *WavWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	_, headerErr := w.file.WriteAt(w.header(), 0)
	syncErr := w.file.Sync()
	return errors.Join(headerErr, syncErr, w.file.Close())
}

func (w *WavWriter) header() []byte {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+w.bytes)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], SampleRate*2)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], w.bytes)
	return h
}

func (w *WavWriter) String() string { return fmt.Sprintf("WAV(%d samples)", w.bytes/2) }
