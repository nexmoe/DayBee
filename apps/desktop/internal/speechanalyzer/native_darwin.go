//go:build darwin && cgo

package speechanalyzer

/*
#include <dlfcn.h>
#include <stdlib.h>
#include <stdint.h>

static int (*speech_available)(void);
static void *(*speech_start)(const char *, const char *, const float *, int64_t, int);
static char *(*speech_poll)(void *);
static void (*speech_cancel)(void *);
static void (*speech_release)(void *);

static int load_speech(const char *path) {
    void *lib = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (!lib) return 0;
    speech_available = dlsym(lib, "daybee_speech_available");
    speech_start = dlsym(lib, "daybee_speech_start");
    speech_poll = dlsym(lib, "daybee_speech_poll");
    speech_cancel = dlsym(lib, "daybee_speech_cancel");
    speech_release = dlsym(lib, "daybee_speech_release");
    return speech_available && speech_start && speech_poll && speech_cancel && speech_release;
}
static int available_speech(void) { return speech_available(); }
static void *start_speech(const char *op, const char *locale, const float *pcm, int64_t n, int rate) {
    return speech_start(op, locale, pcm, n, rate);
}
static char *poll_speech(void *request) { return speech_poll(request); }
static void cancel_speech(void *request) { speech_cancel(request); }
static void release_speech(void *request) { speech_release(request); }
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

var available = sync.OnceValue(func() bool {
	paths := []string{os.Getenv("DAYBEE_SPEECH_LIBRARY")}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "..", "Resources", "lib", "libdaybee-speech.dylib"))
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		value := C.CString(path)
		loaded := C.load_speech(value) != 0
		C.free(unsafe.Pointer(value))
		if loaded {
			return C.available_speech() != 0
		}
	}
	return false
})

func Available() bool { return available() }

func request(ctx context.Context, operation, language string, samples []float32, rate int) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !Available() {
		return Result{}, ErrUnavailable
	}
	op, locale := C.CString(operation), C.CString(language)
	defer C.free(unsafe.Pointer(op))
	defer C.free(unsafe.Pointer(locale))
	var pcm *C.float
	if len(samples) != 0 {
		pcm = (*C.float)(unsafe.Pointer(&samples[0]))
	}
	// Swift copies PCM before returning; no Go pointers survive this call.
	handle := C.start_speech(op, locale, pcm, C.int64_t(len(samples)), C.int(rate))
	if handle == nil {
		return Result{}, errors.New("Unable to start SpeechAnalyzer.")
	}
	defer C.release_speech(handle)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw := C.poll_speech(handle); raw != nil {
			data := C.GoString(raw)
			C.free(unsafe.Pointer(raw))
			var result Result
			if err := json.Unmarshal([]byte(data), &result); err != nil {
				return Result{}, err
			}
			if result.Error != "" {
				return Result{}, errors.New(result.Error)
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			C.cancel_speech(handle)
			return Result{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
