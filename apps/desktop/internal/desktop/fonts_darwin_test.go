package desktop

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledPublicFont(t *testing.T) {
	path, err := installedFontPath()
	if errors.Is(err, errPublicFontUnavailable) {
		t.Skip("Optional static PingFang faces are not installed; the renderer uses system fonts")
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("Installed static font: %s, %v", path, err)
	}
}

func TestFontResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.ttc")
	if err := os.WriteFile(path, []byte("ttcf-fixed-font-data"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := installedFontHandler(path)
	for _, tc := range []struct {
		method, url, rangeHeader string
		status                   int
		body                     string
	}{
		{"GET", "daybee-font://system/pingfang.ttc", "", 200, "ttcf-fixed-font-data"},
		{"GET", "daybee-font://system/pingfang.ttc", "bytes=0-3", 206, "ttcf"},
		{"HEAD", "daybee-font://system/pingfang.ttc", "", 200, ""},
		{"OPTIONS", "daybee-font://system/pingfang.ttc", "", 204, ""},
		{"POST", "daybee-font://system/pingfang.ttc", "", 405, ""},
		{"GET", "daybee-font://system/../installed.ttc", "", 404, "404 page not found\n"},
		{"GET", "daybee-font://other/pingfang.ttc", "", 404, "404 page not found\n"},
	} {
		t.Run(tc.method+tc.url+tc.rangeHeader, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.url, nil)
			r.Header.Set("Range", tc.rangeHeader)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || w.Body.String() != tc.body {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if tc.status < 400 && w.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatal("Font resource lacks CORS header")
			}
		})
	}
}
