package micactivity

/*
#cgo LDFLAGS: -lole32 -luuid -lshell32 -lgdi32 -lversion
#include <stdlib.h>
#include <windows.h>
DWORD *daybeeCapturePIDs(UINT *count, HRESULT *status);
int daybeeProcessPath(DWORD pid, WCHAR *path, DWORD size);
void daybeeApplicationInfo(const WCHAR *path, WCHAR *name, DWORD nameSize, unsigned char *pixels);
*/
import "C"

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"
)

var applicationCache = map[string]Application{}

func NativeScan() ([]Application, error) {
	var count C.UINT
	var status C.HRESULT
	pids := C.daybeeCapturePIDs(&count, &status)
	if pids != nil {
		defer C.free(unsafe.Pointer(pids))
	}
	if status < 0 {
		return nil, fmt.Errorf("Read microphone applications (WASAPI %#x)", uint32(status))
	}
	selfPath, _ := os.Executable()
	apps := []Application{}
	for _, pid := range unsafe.Slice(pids, int(count)) {
		if int(pid) == os.Getpid() || pid == 0 {
			continue
		}
		pathBuffer := make([]uint16, 32768)
		if C.daybeeProcessPath(pid, (*C.WCHAR)(unsafe.Pointer(&pathBuffer[0])), C.DWORD(len(pathBuffer))) == 0 {
			// The process can exit between session enumeration and lookup.
			continue
		}
		path := windowsString(pathBuffer)
		if strings.EqualFold(path, selfPath) {
			continue
		}
		id := strings.ToLower(path)
		app, ok := applicationCache[id]
		if !ok {
			name := make([]uint16, 1024)
			pixels := make([]byte, 32*32*4)
			C.daybeeApplicationInfo((*C.WCHAR)(unsafe.Pointer(&pathBuffer[0])),
				(*C.WCHAR)(unsafe.Pointer(&name[0])), C.DWORD(len(name)), (*C.uchar)(unsafe.Pointer(&pixels[0])))
			app = Application{ID: id, Name: windowsString(name), Icon: encodeIcon(pixels)}
			if app.Name == "" {
				app.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			}
			if len(applicationCache) >= 256 {
				clear(applicationCache)
			}
			applicationCache[id] = app
		}
		app.PID = int(pid)
		apps = append(apps, app)
	}
	return apps, nil
}

func windowsString(value []uint16) string {
	for i, unit := range value {
		if unit == 0 {
			return string(utf16.Decode(value[:i]))
		}
	}
	return string(utf16.Decode(value))
}

func encodeIcon(bgra []byte) string {
	bitmap := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	visible := false
	for i := 0; i < len(bgra); i += 4 {
		alpha := bgra[i+3]
		if alpha > 0 {
			visible = true
			// DrawIconEx writes premultiplied channels; PNG expects straight alpha.
			for channel := 0; channel < 3; channel++ {
				bitmap.Pix[i+channel] = byte(min(255, int(bgra[i+2-channel])*255/int(alpha)))
			}
		}
		bitmap.Pix[i+3] = alpha
	}
	if !visible {
		return ""
	}
	var data bytes.Buffer
	if png.Encode(&data, bitmap) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}
