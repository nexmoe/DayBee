package desktop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type pendingInstallation struct {
	version string
	install func(context.Context, func(int64, int64)) error
}

func (s *DesktopService) updateStatus(fn func(*protocol.AutoUpdateSnapshot)) {
	s.mu.Lock()
	fn(&s.updater)
	s.mu.Unlock()
	s.markDirty()
}
func (s *DesktopService) updateLoop() {
	if s.test || !mygo.Updater.Enabled() {
		s.updateStatus(func(u *protocol.AutoUpdateSnapshot) {
			u.Phase = protocol.UpdateUnavailable
			u.StatusMessage = "Automatic updates are available in packaged builds."
		})
		return
	}
	go s.checkUpdate()
	check := time.NewTicker(6 * time.Hour)
	defer check.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-check.C:
			go s.checkUpdate()
		}
	}
}

func (s *DesktopService) checkUpdate() {
	if !s.checking.CompareAndSwap(false, true) {
		return
	}
	defer s.checking.Store(false)
	s.mu.RLock()
	pending := s.pendingUpdate != nil || s.installing
	s.mu.RUnlock()
	if pending {
		return
	}
	s.updateStatus(func(u *protocol.AutoUpdateSnapshot) {
		u.Phase = protocol.UpdateChecking
		u.ErrorMessage = nil
		u.StatusMessage = "Checking for updates…"
	})
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	update, err := mygo.Updater.Check(ctx)
	if err != nil {
		s.updateFailed(err)
		return
	}
	if update == nil {
		s.updateStatus(func(u *protocol.AutoUpdateSnapshot) {
			u.Phase = protocol.UpdateIdle
			u.StatusMessage = "You are on the latest version."
		})
		return
	}
	s.updateAvailable(&pendingInstallation{version: update.Version, install: update.Install})
}

func (s *DesktopService) updateAvailable(update *pendingInstallation) {
	s.mu.Lock()
	s.pendingUpdate = update
	s.updater.LatestVersion = &update.version
	s.updater.DownloadedVersion = nil
	s.updater.DownloadedVersionReady = false
	s.updater.InstallDeferredUntilIdle = false
	s.updater.ErrorMessage = nil
	s.updater.Phase = protocol.UpdateAvailable
	s.updater.StatusMessage = "Update " + update.version + " is available. Install and restart to update."
	s.mu.Unlock()
	s.markDirty()
}

// InstallUpdate saves the current recording, installs the detected update and relaunches.
func (s *DesktopService) InstallUpdate() error {
	if err := s.available(); err != nil {
		return err
	}
	s.mu.Lock()
	update := s.pendingUpdate
	if s.installing {
		s.mu.Unlock()
		return nil
	}
	if s.quitting.Load() {
		s.mu.Unlock()
		return errors.New("Application is quitting.")
	}
	if update == nil {
		s.mu.Unlock()
		return errors.New("No update is available.")
	}
	s.installing = true
	s.updater.Phase = protocol.UpdateDownloading
	s.updater.ErrorMessage = nil
	s.updater.InstallDeferredUntilIdle = false
	s.updater.StatusMessage = "Preparing update " + update.version + "…"
	s.mu.Unlock()
	s.markDirty()
	// Block new recorder/settings operations while any in-flight operation finishes.
	s.operations.Lock()
	defer s.operations.Unlock()
	if s.quitting.Load() {
		return s.updateFailed(errors.New("Application is quitting."))
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		return s.updateFailed(errors.New("Finish or cancel voiceprint enrollment before installing the update."))
	}
	if s.engine != nil {
		if err := s.engine.Stop(); err != nil {
			return s.updateFailed(fmt.Errorf("saving recording before update: %w", err))
		}
	}
	s.updateStatus(func(u *protocol.AutoUpdateSnapshot) {
		u.StatusMessage = "Downloading update " + update.version + "…"
	})
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()
	err := update.install(ctx, func(done, total int64) {
		if total > 0 {
			s.updateStatus(func(u *protocol.AutoUpdateSnapshot) {
				u.StatusMessage = fmt.Sprintf("Downloading update… %d%%", done*100/total)
			})
		}
	})
	if err != nil {
		return s.updateFailed(err)
	}
	s.mu.Lock()
	s.pendingUpdate = nil
	s.updater.Phase = protocol.UpdateDownloaded
	s.updater.DownloadedVersion = &update.version
	s.updater.DownloadedVersionReady = true
	s.updater.StatusMessage = "Restarting to install update " + update.version + "…"
	s.mu.Unlock()
	s.markDirty()
	s.relaunch()
	return nil
}
func (s *DesktopService) updateFailed(err error) error {
	s.mu.Lock()
	s.installing = false
	message := err.Error()
	s.updater.ErrorMessage = &message
	s.updater.Phase = protocol.UpdateError
	s.updater.StatusMessage = message
	s.mu.Unlock()
	s.markDirty()
	return err
}
