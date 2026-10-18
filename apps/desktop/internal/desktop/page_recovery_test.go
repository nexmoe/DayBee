package desktop

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

type fakeRecoveryPage struct {
	fail    func(*mygo.LoadError)
	gone    func(string)
	reloads int
}

func (p *fakeRecoveryPage) OnDidFailLoad(fn func(*mygo.LoadError)) func() {
	p.fail = fn
	return func() { p.fail = nil }
}

func (p *fakeRecoveryPage) OnRenderProcessGone(fn func(string)) func() {
	p.gone = fn
	return func() { p.gone = nil }
}

func (p *fakeRecoveryPage) Reload() { p.reloads++ }

func TestRenderGoneReloadsOnceUntilManualRefresh(t *testing.T) {
	page := &fakeRecoveryPage{}
	recovery := attachPageRecovery("main", page)
	page.gone("terminated")
	page.gone("terminated")
	if page.reloads != 1 {
		t.Fatalf("automatic reloads = %d", page.reloads)
	}
	recovery.manual()
	page.gone("terminated")
	page.gone("terminated")
	if page.reloads != 3 {
		t.Fatalf("reloads after manual refresh = %d", page.reloads)
	}
}

func TestLoadFailureLogOmitsURL(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	page := &fakeRecoveryPage{}
	attachPageRecovery("compact", page)
	page.fail(&mygo.LoadError{URL: "https://secret.example/private", Code: 7, Description: "interrupted"})
	text := buf.String()
	if !strings.Contains(text, "role=compact") || !strings.Contains(text, "code=7") || !strings.Contains(text, "interrupted") || strings.Contains(text, "secret.example") {
		t.Fatalf("log = %s", text)
	}
}

func TestBothWindowsReloadAndRearmSpentRecovery(t *testing.T) {
	compact, main := &fakeRecoveryPage{}, &fakeRecoveryPage{}
	service := &DesktopService{
		compactRecovery: attachPageRecovery("compact", compact),
		mainRecovery:    attachPageRecovery("main", main),
	}
	compact.gone("terminated")
	main.gone("terminated")
	compact.gone("terminated")
	main.gone("terminated")
	if compact.reloads != 1 || main.reloads != 1 {
		t.Fatalf("spent quota compact=%d main=%d", compact.reloads, main.reloads)
	}
	service.reloadRecoveries()
	if compact.reloads != 2 || main.reloads != 2 {
		t.Fatalf("manual reload compact=%d main=%d", compact.reloads, main.reloads)
	}
	compact.gone("terminated")
	main.gone("terminated")
	compact.gone("terminated")
	main.gone("terminated")
	if compact.reloads != 3 || main.reloads != 3 {
		t.Fatalf("rearmed quota compact=%d main=%d", compact.reloads, main.reloads)
	}
}

func TestReleaseDropsListenersAndClearsBothFields(t *testing.T) {
	compact, main := &fakeRecoveryPage{}, &fakeRecoveryPage{}
	service := &DesktopService{
		compactRecovery: attachPageRecovery("compact", compact),
		mainRecovery:    attachPageRecovery("main", main),
	}
	compact.gone("terminated")
	main.gone("terminated")
	keepCompact, keepMain := compact.gone, main.gone
	service.releasePageRecovery(service.compactRecovery)
	service.releasePageRecovery(service.mainRecovery)
	if service.compactRecovery != nil || service.mainRecovery != nil {
		t.Fatal("recovery fields still set")
	}
	if compact.gone != nil || main.gone != nil || compact.fail != nil || main.fail != nil {
		t.Fatal("listeners still registered")
	}
	keepCompact("terminated")
	keepMain("terminated")
	if compact.reloads != 1 || main.reloads != 1 {
		t.Fatalf("reload after release compact=%d main=%d", compact.reloads, main.reloads)
	}
}
