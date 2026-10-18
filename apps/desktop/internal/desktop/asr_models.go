package desktop

import (
	"errors"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

// PrepareASRModel downloads without changing the active model. A failed download
// leaves both the selected setting and the previous model cache intact.
func (s *DesktopService) PrepareASRModel(id protocol.ASRModel) (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if !protocol.ValidASRModel(id) {
		return s.snapshot(), errors.New("Unknown ASR model")
	}
	// Preparing only writes a separate model cache. Activation is guarded by
	// SaveSettings/Engine.Apply, so recording can continue during a download.
	if s.models != nil {
		if err := s.models.EnsureASRModel(s.ctx, id); err != nil {
			return s.snapshot(), err
		}
	}
	s.markDirty()
	return s.snapshot(), nil
}
