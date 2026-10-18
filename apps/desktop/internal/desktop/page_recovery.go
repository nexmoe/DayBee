package desktop

import (
	"log/slog"
	"sync"

	"github.com/egoist/mygo"
)

type recoverablePage interface {
	OnDidFailLoad(func(*mygo.LoadError)) (off func())
	OnRenderProcessGone(func(reason string)) (off func())
	Reload()
}

type pageRecovery struct {
	role    string
	page    recoverablePage
	mu      sync.Mutex
	auto    bool
	offFail func()
	offGone func()
}

// Recovery handles are owned by the main thread, including initial setup.
func (s *DesktopService) installPageRecovery(role string, window *mygo.Window) {
	if s == nil || window == nil {
		return
	}
	mygo.RunOnMain(func() {
		page := window.Page()
		if page == nil {
			return
		}
		recovery := attachPageRecovery(role, page)
		if role == "main" {
			s.mainRecovery = recovery
		} else {
			s.compactRecovery = recovery
		}
		window.OnClosed(func() { s.releasePageRecovery(recovery) })
	})
}

func (s *DesktopService) releasePageRecovery(recovery *pageRecovery) {
	if s == nil || recovery == nil {
		return
	}
	recovery.release()
	if s.compactRecovery == recovery {
		s.compactRecovery = nil
	}
	if s.mainRecovery == recovery {
		s.mainRecovery = nil
	}
}

func attachPageRecovery(role string, page recoverablePage) *pageRecovery {
	if page == nil {
		return nil
	}
	recovery := &pageRecovery{role: role, page: page, auto: true}
	recovery.offFail = page.OnDidFailLoad(func(err *mygo.LoadError) {
		code, description := 0, ""
		if err != nil {
			code, description = err.Code, err.Description
		}
		slog.Info("page load failed", "role", role, "code", code, "description", description)
	})
	recovery.offGone = page.OnRenderProcessGone(func(reason string) {
		recovery.mu.Lock()
		current := recovery.page
		reload := recovery.auto && current != nil
		recovery.auto = false
		recovery.mu.Unlock()
		slog.Warn("page render process gone", "role", role, "reason", reason, "reload", reload)
		if reload {
			current.Reload()
		}
	})
	return recovery
}

func (r *pageRecovery) manual() {
	if r == nil {
		return
	}
	r.mu.Lock()
	page := r.page
	if page != nil {
		r.auto = true
	}
	r.mu.Unlock()
	if page != nil {
		page.Reload()
	}
}

func (r *pageRecovery) release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.page = nil
	r.auto = false
	offFail, offGone := r.offFail, r.offGone
	r.offFail, r.offGone = nil, nil
	r.mu.Unlock()
	if offFail != nil {
		offFail()
	}
	if offGone != nil {
		offGone()
	}
}

func (s *DesktopService) reloadInterfaces() {
	mygo.RunOnMain(s.reloadRecoveries)
}

func (s *DesktopService) reloadRecoveries() {
	if s.compactRecovery != nil {
		s.compactRecovery.manual()
	}
	if s.mainRecovery != nil {
		s.mainRecovery.manual()
	}
}
