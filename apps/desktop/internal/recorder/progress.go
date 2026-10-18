package recorder

import (
	"github.com/nexmoe/DayBee/apps/desktop/internal/assets"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func (e *Engine) ModelProgress(p assets.Progress) {
	e.patch(func(s *protocol.RecorderStatusSnapshot) {
		s.Downloading = p.Downloading
		s.DownloadMessage = p.Message
		s.DownloadProgress = p.Percent
		s.VADReady = p.VADReady
		s.SenseVoiceReady = p.ASRReady
	})
}
