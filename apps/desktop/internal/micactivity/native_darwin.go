package micactivity

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework CoreAudio -framework AppKit -framework Foundation
#include <stdlib.h>
char *daybeeMicrophoneApps(int *status);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

func NativeScan() ([]Application, error) {
	var status C.int
	data := C.daybeeMicrophoneApps(&status)
	if data != nil {
		defer C.free(unsafe.Pointer(data))
	}
	if status == -1 {
		return nil, ErrUnsupported
	}
	if status != 0 || data == nil {
		return nil, fmt.Errorf("Read microphone applications (Core Audio %d)", int(status))
	}
	var apps []Application
	err := json.Unmarshal([]byte(C.GoString(data)), &apps)
	return apps, err
}
