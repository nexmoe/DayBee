package desktop

import (
	"github.com/egoist/mygo"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func (s *DesktopService) syncTray() {
	if s.tray == nil {
		return
	}
	tip := AppName + " 在后台待命"
	var status protocol.RecorderStatusSnapshot
	if s.engine != nil {
		status = s.engine.Status()
	}
	if status.Recording {
		tip = AppName + " 正在录音 · " + status.Elapsed
	} else if status.Recovering {
		tip = AppName + " 正在恢复录音…"
	}
	if tip == s.trayTip {
		return
	}
	s.trayTip = tip
	s.tray.SetToolTip(tip)
}

func (s *DesktopService) trayMenu() *mygo.Menu {
	snap := s.snapshot()
	status := "空闲"
	if snap.Status.Recording {
		status = "录音中 · " + snap.Status.Elapsed
	} else if snap.Status.Recovering {
		status = "正在恢复录音…"
	}
	action := func(label string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Click: func(_ *mygo.MenuItem, _ *mygo.Window) { go fn() }}
	}
	start := action("开始录音", func() { _, _ = s.StartRecording() })
	start.Disabled = snap.Status.Recording || snap.Status.Recovering
	stop := action("停止录音", func() { _, _ = s.StopRecording() })
	stop.Disabled = !snap.Status.Recording && !snap.Status.Recovering
	login := action("开机自启", func() {
		if s.guard() != nil {
			return
		}
		settings := s.store.Settings()
		settings.Desktop.LaunchAtLogin = !settings.Desktop.LaunchAtLogin
		_, _ = s.SaveSettings(settings)
	})
	login.Type = mygo.MenuItemCheckbox
	login.Checked = snap.Settings.Desktop.LaunchAtLogin
	items := []*mygo.MenuItem{start, stop, mygo.Separator(), login, {Label: "状态：" + status, Disabled: true},
		{Label: "设备：" + snap.Status.DeviceLabel, Disabled: true}, action("打开开发者工具", func() { s.window.Page().OpenDevTools() }), mygo.Separator(),
		{Label: "版本：" + snap.App.Version, Disabled: true}}
	switch snap.Updater.Phase {
	case protocol.UpdateChecking:
		items = append(items, &mygo.MenuItem{Label: "正在检查更新…", Disabled: true})
	case protocol.UpdateDownloading:
		items = append(items, &mygo.MenuItem{Label: snap.Updater.StatusMessage, Disabled: true})
	case protocol.UpdateAvailable:
		items = append(items, action("有新更新 · 立即安装并重启", func() { _ = s.InstallUpdate() }))
	case protocol.UpdateDownloaded:
		items = append(items, &mygo.MenuItem{Label: "更新已安装，正在重启…", Disabled: true})
	case protocol.UpdateError:
		retry := s.checkUpdate
		if snap.Updater.LatestVersion != nil {
			retry = func() { _ = s.InstallUpdate() }
		}
		items = append(items, &mygo.MenuItem{Label: "更新失败", Disabled: true}, action("重试更新", retry))
	case protocol.UpdateIdle:
		items = append(items, &mygo.MenuItem{Label: "已是最新版本", Disabled: true}, action("检查更新", s.checkUpdate))
	default:
		items = append(items, action("检查更新", s.checkUpdate))
	}
	items = append(items, mygo.Separator(), action("打开 DayBee", s.OpenRecorderWindow), action("刷新界面", s.reloadInterfaces), mygo.Separator(), action("退出 "+AppName, mygo.App.Quit))
	return mygo.NewMenu(items)
}
