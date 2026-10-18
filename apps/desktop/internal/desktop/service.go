package desktop

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/nexmoe/DayBee/apps/desktop/internal/permissions"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func (s *DesktopService) Bootstrap() protocol.DesktopSnapshot {
	<-s.ready
	if !s.test {
		s.mu.Lock()
		s.permission = permissions.Status()
		s.mu.Unlock()
	}
	return s.snapshot()
}

func (s *DesktopService) StartRecording() (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	s.captureOps.Lock()
	defer s.captureOps.Unlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		return s.snapshot(), errors.New("Finish or cancel voiceprint enrollment before recording.")
	}
	if s.voiceSetup != nil {
		if err := s.reconcileVoiceprintSetup(s.voiceSetup.Snapshot().Enrolled); err != nil {
			return s.snapshot(), err
		}
	}
	if s.store.Settings().Desktop.VoiceprintSetup == protocol.VoiceprintPending {
		s.voiceStartRequested.Store(true)
		return s.snapshot(), s.OpenMainWindow()
	}
	permission := s.RequestMicrophonePermission()
	if permission.State != protocol.PermissionAuthorized && permission.Supported {
		return s.snapshot(), errors.New(permission.Message)
	}
	err := s.engine.Start()
	s.markDirty()
	return s.snapshot(), err
}
func (s *DesktopService) StopRecording() (protocol.DesktopSnapshot, error) {
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		s.voiceSetup.Cancel()
	}
	s.operations.RLock()
	defer s.operations.RUnlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	s.voiceStartRequested.Store(false)
	err := s.engine.Stop()
	s.markDirty()
	return s.snapshot(), err
}
func (s *DesktopService) RunTranscribe(directory string) (protocol.DesktopSnapshot, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	if err := s.guard(); err != nil {
		return s.snapshot(), err
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		return s.snapshot(), errors.New("Finish or cancel voiceprint enrollment before transcribing.")
	}
	if !filepath.IsAbs(directory) {
		return s.snapshot(), errors.New("Transcription directory must be absolute.")
	}
	err := s.engine.Transcribe(directory)
	s.refreshHistory()
	s.markDirty()
	return s.snapshot(), err
}
func (s *DesktopService) SaveSettings(settings protocol.AppSettings) (protocol.AppSettings, error) {
	s.operations.RLock()
	defer s.operations.RUnlock()
	s.saves.Lock()
	defer s.saves.Unlock()
	defer s.syncVoiceprintAdaptation()
	if err := s.guard(); err != nil {
		return settings, err
	}
	previous := s.store.Settings()
	if previous.Recording.ASRModel != settings.Recording.ASRModel && s.voiceSetup != nil && s.voiceSetup.Active() {
		return previous, errors.New("Finish or cancel voiceprint enrollment before switching ASR models.")
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() && captureSettingsChanged(previous, settings) {
		return previous, errors.New("Finish or cancel voiceprint enrollment before changing audio input settings.")
	}
	saved, err := s.store.Save(settings)
	if err != nil {
		return previous, err
	}
	if err = s.engine.Apply(saved); err != nil {
		_, _ = s.store.Save(previous)
		return previous, err
	}
	s.applySettingsTheme(saved)
	if previous.Recording.OutputDir != saved.Recording.OutputDir {
		s.refreshHistory()
	}
	if s.shouldApplyLoginItem() {
		if err = mygo.App.SetOpenAtLogin(saved.Desktop.LaunchAtLogin); err != nil {
			return saved, err
		}
	}
	s.markDirty()
	return saved, nil
}

func (s *DesktopService) applySettingsTheme(settings protocol.AppSettings) {
	if s != nil && s.settingsTheme != nil {
		s.settingsTheme(settings)
		return
	}
	mygo.Theme.SetSource(mygo.ThemeSource(settings.Desktop.Theme))
}

func captureSettingsChanged(a, b protocol.AppSettings) bool {
	x, y := a.Recording, b.Recording
	return x.Device != y.Device || x.EchoCancellation != y.EchoCancellation ||
		x.RecordSystemAudio != y.RecordSystemAudio || x.AutoSwitchDevice != y.AutoSwitchDevice ||
		x.ExcludeDeviceKeywords != y.ExcludeDeviceKeywords
}
func (s *DesktopService) RequestMicrophonePermission() protocol.MicrophonePermissionStatus {
	<-s.ready
	if !s.test {
		permission := permissions.Request()
		s.mu.Lock()
		s.permission = permission
		s.mu.Unlock()
	}
	s.markDirty()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permission
}
func (s *DesktopService) OpenMicrophoneSettings() bool {
	target := "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone"
	if runtime.GOOS == "windows" {
		target = "ms-settings:privacy-microphone"
	}
	return mygo.Shell.OpenExternal(target) == nil
}
func (s *DesktopService) PickDirectory(defaultPath *string) (*string, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	options := mygo.OpenDialogOptions{Parent: mygo.FocusedWindow(), Directory: true, CreateDirectories: true}
	if defaultPath != nil {
		options.DefaultPath = *defaultPath
	}
	paths, err := mygo.Dialog.Open(options)
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return &paths[0], nil
}
func (s *DesktopService) OpenRecordingFolder(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("Recording folder must be absolute.")
	}
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		path = filepath.Dir(path)
	}
	return mygo.Shell.OpenPath(path)
}
func (s *DesktopService) OpenExternal(target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("Unsupported external URL.")
	}
	return mygo.Shell.OpenExternal(target)
}
func (s *DesktopService) CloseWindow(role string) {
	if window := s.windowForRole(role); window != nil {
		window.Hide()
	}
}
func (s *DesktopService) MinimizeWindow(role string) {
	if window := s.windowForRole(role); window != nil {
		window.Minimize()
	}
}
func (s *DesktopService) SetWindowPinned(pinned bool) protocol.DesktopSnapshot {
	<-s.ready
	s.mu.Lock()
	s.pinned = pinned
	s.mu.Unlock()
	if s.window != nil {
		s.window.SetAlwaysOnTop(pinned)
	}
	s.markDirty()
	return s.snapshot()
}
