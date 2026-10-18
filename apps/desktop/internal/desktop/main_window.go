package desktop

import (
	"errors"
	"os"
	"runtime"

	"github.com/egoist/mygo"
)

// The main window is created once, on demand. Its lifetime never owns recording.
func (s *DesktopService) OpenMainWindow() error {
	<-s.ready
	if s.quitting.Load() {
		return errors.New("Application is quitting.")
	}
	mygo.RunOnMain(func() {
		if s.mainWindow == nil {
			options := mygo.WindowOptions{
				Title: AppName + " — Workspace", URL: "/?window=main",
				Width: 1000, Height: 760, MinWidth: 720, MinHeight: 540, Hidden: true,
			}
			if s.test && os.Getenv("DAYBEE_TEST_VISUAL") == "1" {
				options.Page.PreloadScript = visualPreload
			}
			if runtime.GOOS == "darwin" {
				options.TitleBarStyle = mygo.TitleBarHidden
				// Match --workspace-leading-inset in window-layout.css.
				options.TrafficLightPosition = &mygo.Point{X: 16, Y: 14}
				options.Transparent = true
				options.Vibrancy = mygo.VibrancyPopover
			} else {
				options.Frameless = true
				options.Vibrancy = mygo.VibrancyAcrylic
				options.BackgroundColor = "#ffffff"
			}
			window := mygo.NewWindow(options)
			s.mainWindow = window
			s.trackWindowUpdates("main", window)
			installWindowCompatibility(window, 38)
			s.installPageRecovery("main", window)
			if runtime.GOOS == "windows" {
				window.SetMenu(mygo.NewMenu(nil))
			}
			window.Center()
			window.OnClose(func(event *mygo.CloseEvent) {
				if !s.quitReady.Load() {
					event.PreventDefault()
					window.Hide()
				}
			})
			window.OnReadyToShow(func() { s.showMainWindow() })
		} else {
			s.showMainWindow()
		}
	})
	return nil
}

func (s *DesktopService) OpenRecorderWindow() {
	s.reveal()
}

func (s *DesktopService) showMainWindow() {
	if s.backgroundTest() {
		return
	}
	if mygo.App.IsHidden() {
		mygo.App.Show()
	}
	if s.mainWindow.IsMinimized() {
		s.mainWindow.Restore()
	}
	s.mainWindow.Show()
	s.mainWindow.Focus()
}

// Window controls target their own role, even when a second window has focus.
func (s *DesktopService) windowForRole(role string) *mygo.Window {
	var window *mygo.Window
	mygo.RunOnMain(func() {
		if role == "main" {
			window = s.mainWindow
		} else if role == "compact" {
			window = s.window
		}
	})
	return window
}
