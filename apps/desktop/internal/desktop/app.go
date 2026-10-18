package desktop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/nexmoe/eve/apps/desktop/internal/asr"
	"github.com/nexmoe/eve/apps/desktop/internal/asrmodel"
	"github.com/nexmoe/eve/apps/desktop/internal/assets"
	"github.com/nexmoe/eve/apps/desktop/internal/audio"
	"github.com/nexmoe/eve/apps/desktop/internal/config"
	"github.com/nexmoe/eve/apps/desktop/internal/micactivity"
	"github.com/nexmoe/eve/apps/desktop/internal/permissions"
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"github.com/nexmoe/eve/apps/desktop/internal/recorder"
	"github.com/nexmoe/eve/apps/desktop/internal/voiceprint"
)

var SnapshotEvent = mygo.NewEvent[protocol.DesktopSnapshot]("desktop:snapshot")
var StatusEvent = mygo.NewEvent[protocol.DesktopStatusSnapshot]("desktop:status")
var WaveformEvent = mygo.NewEvent[protocol.WaveformFrame]("desktop:waveform")

type engine interface {
	Start() error
	Stop() error
	Apply(protocol.AppSettings) error
	Status() protocol.RecorderStatusSnapshot
	Devices() []protocol.DeviceInfo
	Transcribe(string) error
	Busy() bool
	Suspend() error
	Resume() error
	Close() error
	HistoryChanges() <-chan struct{}
}

// DesktopService owns work for the entire process, independent of page contexts.
type DesktopService struct {
	mu                  sync.RWMutex
	operations          sync.RWMutex
	saves               sync.Mutex
	captureOps          sync.Mutex
	ready               chan struct{}
	ctx                 context.Context
	cancel              context.CancelFunc
	store               *config.Store
	engine              engine
	models              *assets.Manager
	microphone          *micactivity.Monitor
	microphoneError     *string
	microphoneActivity  protocol.MicrophoneActivitySnapshot
	window              *mygo.Window
	mainWindow          *mygo.Window
	compactRecovery     *pageRecovery
	mainRecovery        *pageRecovery
	tray                *mygo.Tray
	trayTip             string
	windows             windowUpdates
	permission          protocol.MicrophonePermissionStatus
	history             []protocol.RecordingHistoryItem
	defaultOutput       string
	legacyOutputs       []string
	updater             protocol.AutoUpdateSnapshot
	pinned              bool
	initError           error
	dirty               atomic.Bool
	historyVersion      atomic.Uint64
	quitting            atomic.Bool
	quitReady           atomic.Bool
	checking            atomic.Bool
	revealUntil         atomic.Int64
	pendingUpdate       *pendingInstallation
	relaunch            func()
	installing          bool
	test                bool
	fixture             *protocol.DesktopSnapshot
	settingsTheme       func(protocol.AppSettings)
	voiceSetup          voiceprintSetup
	voiceprints         *voiceprint.Manager
	voiceStartRequested atomic.Bool
}

func New() *DesktopService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &DesktopService{ready: make(chan struct{}), ctx: ctx, cancel: cancel, test: os.Getenv("DAYBEE_E2E_TEST") == "1",
		history: []protocol.RecordingHistoryItem{}, permission: permissions.Status(), relaunch: mygo.App.Relaunch}
	s.updater = protocol.AutoUpdateSnapshot{CurrentVersion: mygo.App.Version(), Phase: protocol.UpdateIdle, StatusMessage: "Automatic updates are ready."}
	return s
}

// Run registers lifecycle hooks without performing work during binding generation.
func Run(s *DesktopService) error {
	if err := registerFontCompatibility(); err != nil {
		slog.Error("Legacy font compatibility", "error", err)
	}
	mygo.Bind(s)
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnSecondInstance(func(_ []string, _ string) { s.reveal() })
	mygo.App.WhenReady(func() { go s.initialize() })
	mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
		if s.quitReady.Load() {
			return
		}
		event.PreventDefault()
		if !s.quitting.CompareAndSwap(false, true) {
			return
		}
		go func() {
			<-s.ready
			s.cancel()
			if s.voiceSetup != nil {
				s.voiceSetup.Close()
			}
			if s.microphone != nil {
				if err := s.microphone.Close(time.Now()); err != nil {
					slog.Error("Microphone activity shutdown", "error", err)
				}
			}
			if s.engine != nil {
				if err := s.engine.Close(); err != nil {
					slog.Error("Recorder shutdown", "error", err)
				}
			}
			if s.voiceprints != nil {
				s.voiceprints.Close()
			}
			s.quitReady.Store(true)
			mygo.App.Quit()
		}()
	})
	power := make(chan bool, 16)
	mygo.Power.OnSuspend(func() { power <- true })
	mygo.Power.OnResume(func() { power <- false })
	mygo.Power.OnUnlockScreen(func() { power <- false })
	go s.powerLoop(power)
	if os.Getenv("MYGO_GENERATE") == "" && !s.test && !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)
	go func() {
		select {
		case <-shutdown:
			mygo.App.Quit()
		case <-s.ctx.Done():
		}
	}()
	return mygo.App.Run()
}

func (s *DesktopService) powerLoop(events <-chan bool) {
	select {
	case <-s.ready:
	case <-s.ctx.Done():
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		case suspended := <-events:
			slog.Info("Power or session transition", "suspended", suspended)
			if s.microphone != nil {
				if suspended {
					if err := s.microphone.Suspend(time.Now()); err != nil {
						slog.Error("Microphone activity suspend", "error", err)
					}
				} else {
					s.microphone.Resume()
				}
				s.markDirty()
			}
			s.operations.RLock()
			if s.engine != nil && !s.quitting.Load() {
				var err error
				if suspended {
					err = s.engine.Suspend()
				} else {
					err = s.engine.Resume()
				}
				if err != nil {
					slog.Error("Power transition", "suspended", suspended, "error", err)
				}
			}
			s.operations.RUnlock()
			if suspended && s.voiceSetup != nil {
				s.voiceSetup.Cancel()
			}
		}
	}
}

func (s *DesktopService) initialize() {
	err := s.initializeEngine()
	s.mu.Lock()
	s.initError = err
	s.mu.Unlock()
	if err != nil {
		slog.Error("Desktop initialization", "error", err)
	}
	close(s.ready)
	if err := s.createWindow(); err != nil {
		slog.Error("Window initialization", "error", err)
		mygo.App.Quit()
		return
	}
	s.markDirty()
	go s.publishLoop()
	go s.microphoneActivityLoop()
	go s.updateLoop()
	if s.initError == nil && !s.quitting.Load() {
		settings := s.store.Settings()
		if settings.Desktop.VoiceprintSetup == protocol.VoiceprintPending {
			_ = s.OpenMainWindow()
		} else if settings.Desktop.StartRecordingOnLaunch && !s.test {
			_, _ = s.StartRecording()
		}
	}
	if s.test {
		go s.nativeTestServer()
	}
}

func (s *DesktopService) initializeEngine() error {
	appData, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		return err
	}
	documents, err := mygo.App.Path(mygo.PathDocuments)
	if err != nil {
		return err
	}
	paths := s.dataPaths(appData, documents)
	root, documents, legacy := paths.root, paths.documents, paths.legacy
	mygo.App.SetPath(mygo.PathUserData, root)
	// History must still include the old default folder after a user chooses another output path.
	s.defaultOutput = filepath.Join(documents, "DayBee")
	if !s.isolated() {
		s.legacyOutputs = []string{filepath.Join(documents, "RecBee"), filepath.Join(documents, "Eve Recorder")}
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(root, "logs", "main.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, logFile), nil)))
	slog.Info("Application data", "directory", root, "isolated", s.isolated(), "development", mygo.IsDev())
	store, err := config.Open(root, documents, legacy, !s.test)
	if err != nil {
		return err
	}
	s.store = store
	if err := importLegacyState(root, legacy); err != nil {
		return err
	}
	s.initializeMicrophoneActivity(root)
	settings := store.Settings()
	mygo.Theme.SetSource(mygo.ThemeSource(settings.Desktop.Theme))
	if s.shouldApplyLoginItem() {
		if err := mygo.App.SetOpenAtLogin(settings.Desktop.LaunchAtLogin); err != nil {
			slog.Error("Login item", "error", err)
		}
	}
	if s.test {
		s.permission = protocol.MicrophonePermissionStatus{Message: "麦克风权限已授权。", State: protocol.PermissionAuthorized, Supported: true}
		s.engine = newTestEngine(settings, s.markDirty)
		s.voiceSetup = newTestVoiceprintSetup(s)
		return s.loadTestVoiceprint(root)
	}
	manager := assets.New(root, legacy, func(progress assets.Progress) {
		s.mu.RLock()
		current := s.engine
		s.mu.RUnlock()
		if e, ok := current.(*recorder.Engine); ok {
			e.ModelProgress(progress)
		}
	})
	s.models = manager
	if err := asr.CheckVersion(); err != nil {
		return err
	}
	capture, err := audio.NewNativeCapture()
	if err != nil {
		return err
	}
	voiceprints, profileErr := voiceprint.New(root, manager.VoiceprintPath)
	if profileErr != nil {
		slog.Warn("Voiceprint profile unavailable; transcription continues", "error", profileErr)
	}
	if err := s.reconcileVoiceprintSetup(voiceprints.Enrolled()); err != nil {
		slog.Warn("Could not reconcile saved voiceprint setup", "error", err)
	}
	settings = store.Settings()
	s.voiceprints = voiceprints
	s.syncVoiceprintAdaptation()
	engine := recorder.New(settings, capture, manager, asr.Sherpa{}, s.markDirty, s.windows.waveform)
	engine.SetWaveformEnabled(false)
	engine.SetVoiceprint(voiceprints)
	s.engine = engine
	s.voiceSetup = newVoiceprintEnrollment(s, manager, voiceprints)
	if voiceprints.Enrolled() {
		go func() {
			if err := manager.EnsureVoiceprint(s.ctx); err != nil {
				slog.Warn("Voiceprint model unavailable; transcription continues", "error", err)
			}
		}()
	}
	return nil
}

func (s *DesktopService) markDirty() { s.dirty.Store(true) }
func (s *DesktopService) publishLoop() {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	history := time.NewTicker(5 * time.Minute)
	defer history.Stop()
	var changes <-chan struct{}
	if s.engine != nil {
		changes = s.engine.HistoryChanges()
	}
	s.refreshHistory()
	s.publishSnapshot()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-history.C:
			s.refreshHistory()
			s.markDirty()
		case <-changes:
			s.refreshHistory()
			s.publishSnapshot()
		case <-tick.C:
			if s.dirty.Swap(false) {
				s.publishSnapshot()
			}
		}
	}
}

func (s *DesktopService) refreshHistory() {
	if s.store == nil {
		return
	}
	settings := s.store.Settings()
	h := recorder.History(s.recordingRoots(settings.Recording.OutputDir))
	if s.microphone != nil {
		// Storage errors remain visible in the activity snapshot and polling log.
		_ = s.microphone.ReconcileHistory(h, time.Now())
		s.refreshMicrophoneSnapshot()
	}
	s.mu.Lock()
	s.history = h
	s.mu.Unlock()
	s.historyVersion.Add(1)
}

func (s *DesktopService) publishSnapshot() {
	s.syncTray()
	if s.windows.active.Load() {
		s.windows.publish(s.snapshot(), s.historyVersion.Load())
	}
}

func (s *DesktopService) snapshot() protocol.DesktopSnapshot {
	s.mu.RLock()
	if s.test && s.fixture != nil {
		snapshot := *s.fixture
		s.mu.RUnlock()
		return snapshot
	}
	snapshot := protocol.DesktopSnapshot{App: protocol.AppInfoSnapshot{Name: AppName, RepositoryURL: "https://github.com/nexmoe/eve", Version: mygo.App.Version()},
		Permission: s.permission, History: append([]protocol.RecordingHistoryItem{}, s.history...), Updater: s.updater, WindowPinned: s.pinned,
		Settings: protocol.DefaultSettings(""), Status: protocol.DefaultStatus(), Devices: []protocol.DeviceInfo{}}
	err := s.initError
	s.mu.RUnlock()
	snapshot.MicrophoneActivity = s.microphoneSnapshot()
	snapshot.Voiceprint = s.voiceprintSnapshot()
	if s.store != nil {
		snapshot.Settings = s.store.Settings()
	}
	if s.engine != nil {
		snapshot.Status = s.engine.Status()
		snapshot.Devices = s.engine.Devices()
		snapshot.EngineReady = true
	}
	snapshot.ASRModels.Items = []protocol.ASRModelSnapshot{}
	if s.models != nil {
		snapshot.ASRModels = s.models.Models()
	} else {
		for _, model := range asrmodel.All() {
			snapshot.ASRModels.Items = append(snapshot.ASRModels.Items, protocol.ASRModelSnapshot{ID: model.ID, Name: model.Name, EstimatedBytes: model.EstimatedBytes, Ready: s.test})
		}
	}
	if s.engine != nil {
		snapshot.ASRModels.Busy = s.engine.Busy()
	}
	if s.voiceSetup != nil && s.voiceSetup.Active() {
		snapshot.ASRModels.Busy = true
	}
	if err != nil {
		message := err.Error()
		snapshot.Status.Error = &message
		snapshot.Status.StatusMessage = message
	}
	return snapshot
}

func (s *DesktopService) available() error { <-s.ready; return s.initError }
func (s *DesktopService) guard() error {
	if err := s.available(); err != nil {
		return err
	}
	if s.quitting.Load() {
		return errors.New("Application is quitting.")
	}
	s.mu.RLock()
	installing := s.installing
	s.mu.RUnlock()
	if installing {
		return errors.New("Application is updating.")
	}
	return nil
}
