package desktop

/*
#cgo LDFLAGS: -framework CoreText -framework CoreFoundation
#include <CoreText/CoreText.h>
#include <stdlib.h>

// Resolve the installed public faces, never a version-specific asset path.
// WebKit's installed-name lookup can choose PingFang UI despite these names.
static char *daybeePublicFontPath(void) {
    const CFStringRef names[] = {
        CFSTR("PingFangSC-Regular"), CFSTR("PingFangSC-Medium"), CFSTR("PingFangSC-Semibold")
    };
    CFURLRef collection = NULL;
    for (int i = 0; i < 3; i++) {
        CTFontRef font = CTFontCreateWithName(names[i], 12, NULL);
        if (!font) goto fail;
        CFStringRef name = CTFontCopyPostScriptName(font);
        CFArrayRef axes = CTFontCopyVariationAxes(font);
        CFTypeRef url = CTFontCopyAttribute(font, kCTFontURLAttribute);
        int valid = name && CFEqual(name, names[i]) && (!axes || CFArrayGetCount(axes) == 0)
            && url && CFGetTypeID(url) == CFURLGetTypeID();
        if (valid && collection) valid = CFEqual(collection, url);
        if (valid && !collection) collection = (CFURLRef)CFRetain(url);
        if (url) CFRelease(url);
        if (axes) CFRelease(axes);
        if (name) CFRelease(name);
        CFRelease(font);
        if (!valid) goto fail;
    }
    CFStringRef path = CFURLCopyFileSystemPath(collection, kCFURLPOSIXPathStyle);
    CFRelease(collection);
    if (!path) return NULL;
    CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength(path), kCFStringEncodingUTF8) + 1;
    char *result = malloc(size);
    if (result && !CFStringGetCString(path, result, size, kCFStringEncodingUTF8)) {
        free(result);
        result = NULL;
    }
    CFRelease(path);
    return result;
fail:
    if (collection) CFRelease(collection);
    return NULL;
}
*/
import "C"

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/egoist/mygo"
)

var errPublicFontUnavailable = errors.New("public static PingFang faces are unavailable")

func registerFontCompatibility() error {
	path, err := installedFontPath()
	if errors.Is(err, errPublicFontUnavailable) {
		slog.Info("Static PingFang is unavailable; using system font fallback")
		return nil
	}
	if err != nil {
		return err
	}
	return mygo.Protocol.Handle("daybee-font", installedFontHandler(path))
}

func installedFontPath() (string, error) {
	path := C.daybeePublicFontPath()
	if path == nil {
		return "", errPublicFontUnavailable
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}

func installedFontHandler(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "system" || r.URL.Path != "/pingfang.ttc" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "font/collection")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
	})
}
