package desktop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func updaterService() *DesktopService {
	ready := make(chan struct{})
	close(ready)
	return &DesktopService{ready: ready, ctx: context.Background(), relaunch: func() {}}
}

func TestDetectedUpdateWaitsForUser(t *testing.T) {
	for _, recording := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "recording"}[recording], func(t *testing.T) {
			s := updaterService()
			s.engine = newTestEngine(protocol.DefaultSettings(t.TempDir()), func() {})
			if recording {
				if err := s.engine.Start(); err != nil {
					t.Fatal(err)
				}
			}
			var installed, restarted int
			s.relaunch = func() { restarted++ }
			s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(context.Context, func(int64, int64)) error {
				installed++
				return nil
			}})
			// A periodic or tray check must not hide the outstanding update.
			s.checkUpdate()
			if installed != 0 || restarted != 0 || s.engine.Status().Recording != recording {
				t.Fatal("detection changed the running application")
			}
			if s.updater.Phase != protocol.UpdateAvailable || s.updater.LatestVersion == nil || *s.updater.LatestVersion != "1.0.0" || s.updater.DownloadedVersion != nil || s.updater.DownloadedVersionReady {
				t.Fatal("detected update was not exposed as available", s.updater)
			}
		})
	}
}

func TestExplicitUpdateSavesRecordingAndRestartsOnce(t *testing.T) {
	s := updaterService()
	s.engine = newTestEngine(protocol.DefaultSettings(t.TempDir()), func() {})
	if err := s.engine.Start(); err != nil {
		t.Fatal(err)
	}
	var installed, restarted int
	s.relaunch = func() { restarted++ }
	s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(_ context.Context, progress func(int64, int64)) error {
		if s.engine.Status().Recording {
			t.Fatal("recording was not saved before installation")
		}
		if err := s.guard(); err == nil {
			t.Fatal("new recording/settings operations allowed during update")
		}
		progress(50, 100)
		installed++
		return nil
	}})
	if err := s.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	if installed != 1 || restarted != 1 || s.updater.Phase != protocol.UpdateDownloaded || !s.updater.DownloadedVersionReady || s.pendingUpdate != nil {
		t.Fatal("explicit update did not install and restart exactly once", s.updater)
	}
}

func TestExplicitUpdateWaitsForOperationBarrier(t *testing.T) {
	s := updaterService()
	var installed atomic.Int64
	s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(context.Context, func(int64, int64)) error {
		installed.Add(1)
		return nil
	}})
	s.operations.RLock()
	done := make(chan error, 1)
	go func() { done <- s.InstallUpdate() }()
	deadline := time.Now().Add(time.Second)
	var started bool
	for time.Now().Before(deadline) {
		s.mu.RLock()
		started = s.installing
		s.mu.RUnlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	before := installed.Load()
	s.operations.RUnlock()
	select {
	case err := <-done:
		if err != nil || !started || before != 0 || installed.Load() != 1 {
			t.Fatal("installation bypassed or failed to resume after an active operation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("installation did not resume after operation completed")
	}
}

func TestConcurrentUpdateClicksInstallOnce(t *testing.T) {
	s := updaterService()
	entered, release := make(chan struct{}), make(chan struct{})
	var installed, restarted atomic.Int64
	s.relaunch = func() { restarted.Add(1) }
	s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(context.Context, func(int64, int64)) error {
		installed.Add(1)
		close(entered)
		<-release
		return nil
	}})
	done := make(chan error, 1)
	go func() { done <- s.InstallUpdate() }()
	<-entered
	err := s.InstallUpdate()
	close(release)
	if first := <-done; err != nil || first != nil || installed.Load() != 1 || restarted.Load() != 1 {
		t.Fatal("concurrent clicks installed more than once", err, first)
	}
}

func TestUpdateFailureKeepsRunningApplicationAndAllowsRetry(t *testing.T) {
	s := updaterService()
	var installed, restarted int
	s.relaunch = func() { restarted++ }
	s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(context.Context, func(int64, int64)) error {
		installed++
		if installed == 1 {
			return errors.New("signature rejected")
		}
		return nil
	}})
	if err := s.InstallUpdate(); err == nil {
		t.Fatal("install error was not returned to the UI")
	}
	if restarted != 0 || s.installing || s.pendingUpdate == nil || s.updater.Phase != protocol.UpdateError || s.updater.ErrorMessage == nil || *s.updater.ErrorMessage != "signature rejected" {
		t.Fatal("failed update restarted app or lost retry state", s.updater)
	}
	if err := s.guard(); err != nil {
		t.Fatal("failed update left the running application blocked", err)
	}
	if err := s.InstallUpdate(); err != nil || installed != 2 || restarted != 1 || s.updater.ErrorMessage != nil {
		t.Fatal("retry did not recover", err, s.updater)
	}
}

type stopFailingEngine struct{ engine }

func (e stopFailingEngine) Stop() error { return errors.New("recording could not be saved") }

func TestUpdateDoesNotInstallIfRecordingCannotBeSaved(t *testing.T) {
	s := updaterService()
	s.engine = stopFailingEngine{}
	s.updateAvailable(&pendingInstallation{version: "1.0.0", install: func(context.Context, func(int64, int64)) error {
		t.Fatal("installed before recording was saved")
		return nil
	}})
	s.relaunch = func() { t.Fatal("restarted before recording was saved") }
	if err := s.InstallUpdate(); err == nil || s.installing || s.pendingUpdate == nil || s.updater.Phase != protocol.UpdateError {
		t.Fatal("recording save error was not recoverable", err)
	}
}

func TestUpdateWithoutPendingVersionReturnsError(t *testing.T) {
	s := updaterService()
	if err := s.InstallUpdate(); err == nil || s.installing {
		t.Fatal("missing update was accepted")
	}
}
