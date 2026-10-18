package recorder

import (
	"sync/atomic"

	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// Each source owns its own VAD, offsets and archive. Only the bounded ASR
// worker is shared; samples from different sources never enter the same VAD.
type audioStream struct {
	source    protocol.AudioSource
	vad       asr.VAD
	segment   *segment
	remainder []float32
	currentID atomic.Uint64
}

func (e *Engine) streams() []*audioStream {
	streams := []*audioStream{&e.audioStream}
	if e.system != nil {
		streams = append(streams, e.system)
	}
	return streams
}

func (e *Engine) configureSystemAudio(settings protocol.AppSettings) error {
	if !settings.Recording.RecordSystemAudio {
		if e.system != nil {
			e.system.currentID.Store(0)
			e.system.vad.Close()
			e.system = nil
		}
		return nil
	}
	if e.system == nil {
		vad, err := e.runtime.NewVAD(e.assets.VADPath())
		if err != nil {
			return err
		}
		e.system = &audioStream{source: protocol.SourceSystem, vad: vad}
	}
	return nil
}

func (e *Engine) closeStreams() {
	for _, stream := range e.streams() {
		if stream.vad != nil {
			stream.vad.Close()
		}
		stream.vad, stream.segment, stream.remainder = nil, nil, nil
	}
	e.system = nil
}
