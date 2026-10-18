package desktop

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

// Tests drive the real WKWebView/WebView2 via Page.Eval and CapturePage. The
// private loopback endpoint is never enabled in ordinary development/releases.
func (s *DesktopService) nativeTestServer() {
	token := os.Getenv("DAYBEE_TEST_TOKEN")
	port := os.Getenv("DAYBEE_TEST_PORT")
	if !s.test || len(token) < 32 || os.Getenv("DAYBEE_STORE_DIR") == "" || port == "" {
		return
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		slog.Error("Native test server", "error", err)
		return
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if r.URL.Path == "/open-main" {
			if err := s.OpenMainWindow(); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			_, _ = fmt.Fprint(w, "null")
			return
		}
		role := r.URL.Query().Get("window")
		if role == "" {
			role = "compact"
		}
		window := s.windowForRole(role)
		if window == nil {
			http.Error(w, "Window unavailable", 409)
			return
		}
		switch r.URL.Path {
		case "/speech":
			var speech struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&speech); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			mock, ok := s.engine.(*testEngine)
			if !ok {
				http.Error(w, "Mock engine required", 409)
				return
			}
			if err := mock.completeSpeech(speech.Text); err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			_, _ = fmt.Fprint(w, "null")
		case "/waveform":
			var frame protocol.WaveformFrame
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&frame); err != nil || len(frame.Spectrum) != 128 {
				http.Error(w, "128 spectrum values required", 400)
				return
			}
			for _, value := range frame.Spectrum {
				if value < 0 || value > 255 {
					http.Error(w, "Invalid spectrum value", 400)
					return
				}
			}
			WaveformEvent.Broadcast(frame)
			_, _ = fmt.Fprint(w, "null")
		case "/fixture":
			var snapshot protocol.DesktopSnapshot
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&snapshot); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if snapshot.Voiceprint.Phase == "" {
				snapshot.Voiceprint = protocol.DefaultVoiceprint()
			}
			if snapshot.Settings.Desktop.VoiceprintSetup == "" {
				snapshot.Settings.Desktop.VoiceprintSetup = protocol.VoiceprintSkipped
			}
			s.mu.Lock()
			s.fixture = &snapshot
			s.mu.Unlock()
			mygo.Theme.SetSource(mygo.ThemeSource(snapshot.Settings.Desktop.Theme))
			SnapshotEvent.Broadcast(snapshot)
			_, _ = fmt.Fprint(w, "null")
		case "/eval":
			var req struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			value, err := window.Page().Eval(req.Code)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(value)
		case "/capture":
			data, err := window.CapturePage()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(data)
		case "/snapshot":
			_ = json.NewEncoder(w).Encode(s.snapshot())
		case "/window":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": window.ID(), "count": len(mygo.Windows()), "visible": window.IsVisible(), "focused": window.IsFocused(), "appHidden": mygo.App.IsHidden(), "bounds": window.Bounds(), "display": mygo.Screen.PrimaryDisplay(), "tray": s.tray.Bounds(), "native": nativeWindowState(window)})
		case "/hide-app":
			mygo.App.Hide()
			_, _ = fmt.Fprint(w, "null")
		case "/hide":
			window.Hide()
			_, _ = fmt.Fprint(w, "null")
		case "/show":
			if role == "main" {
				_ = s.OpenMainWindow()
			} else {
				s.reveal()
			}
			_, _ = fmt.Fprint(w, "null")
		case "/reload":
			window.Page().Reload()
			_, _ = fmt.Fprint(w, "null")
		default:
			http.NotFound(w, r)
		}
	})
	go func() { <-s.ctx.Done(); _ = server.Close() }()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		slog.Error("Native test server", "error", err)
	}
}
