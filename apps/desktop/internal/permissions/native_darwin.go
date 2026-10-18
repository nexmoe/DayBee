package permissions

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework AVFoundation -framework Foundation
#import <AVFoundation/AVFoundation.h>
#import <Foundation/Foundation.h>
static int microphoneStatus(void) {
    return (int)[AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
}
static void microphoneRequest(void) {
    @autoreleasepool {
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        [AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
            dispatch_semaphore_signal(done);
        }];
        dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
        #if !OS_OBJECT_USE_OBJC
        dispatch_release(done);
        #endif
    }
}
*/
import "C"

import "github.com/nexmoe/eve/apps/desktop/internal/protocol"

func nativeStatus() protocol.MicrophonePermissionState {
	switch C.microphoneStatus() {
	case 0:
		return protocol.PermissionNotDetermined
	case 1:
		return protocol.PermissionRestricted
	case 2:
		return protocol.PermissionDenied
	case 3:
		return protocol.PermissionAuthorized
	default:
		return protocol.PermissionUnsupported
	}
}
func nativeRequest() { C.microphoneRequest() }
