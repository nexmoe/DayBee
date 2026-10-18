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
	recovery := attachPageRecovery("compact", page)
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

func TestRefreshReloadsAndRearmsCompactRecovery(t *testing.T) {
	page := &fakeRecoveryPage{}
	service := &DesktopService{compactRecovery: attachPageRecovery("compact", page)}
	page.gone("terminated")
	page.gone("terminated")
	if page.reloads != 1 {
		t.Fatalf("spent quota reloads=%d", page.reloads)
	}
	service.reloadRecoveries()
	if page.reloads != 2 {
		t.Fatalf("manual reloads=%d", page.reloads)
	}
	page.gone("terminated")
	page.gone("terminated")
	if page.reloads != 3 {
		t.Fatalf("rearmed quota reloads=%d", page.reloads)
	}
}

func TestReleaseDropsListenersAndClearsRecovery(t *testing.T) {
	page := &fakeRecoveryPage{}
	service := &DesktopService{compactRecovery: attachPageRecovery("compact", page)}
	page.gone("terminated")
	keepGone := page.gone
	service.releasePageRecovery(service.compactRecovery)
	if service.compactRecovery != nil || page.gone != nil || page.fail != nil {
		t.Fatal("recovery or listeners still registered")
	}
	keepGone("terminated")
	if page.reloads != 1 {
		t.Fatalf("reloads after release=%d", page.reloads)
	}
}
