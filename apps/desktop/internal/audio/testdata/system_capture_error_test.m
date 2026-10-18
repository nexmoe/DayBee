#import "../capture_system_darwin.m"
#import "../capture_system_tap_objc.h"
#include <assert.h>
#include <string.h>

static void verifyStreamError(NSInteger code, int expectedFailure) {
    DayBeeSystemCapture *capture = [[DayBeeSystemCapture alloc] init];
    daybeeVoiceBufferInit(&capture->buffer);
    atomic_init(&capture->stopping, false);
    @autoreleasepool {
        NSError *underlying = [NSError errorWithDomain:NSOSStatusErrorDomain code:-50
            userInfo:@{NSLocalizedDescriptionKey: @"Native audio route failed"}];
        NSError *error = [NSError errorWithDomain:SCStreamErrorDomain code:code userInfo:@{
            NSLocalizedDescriptionKey: @"System audio stream interrupted",
            NSLocalizedFailureReasonErrorKey: @"The capture connection was invalidated",
            NSUnderlyingErrorKey: underlying
        }];
        [capture stream:capture->stream didStopWithError:error];
    }
    // The delegate must retain the error after its callback's autorelease pool.
    float samples[8];
    int failure = 0;
    assert(daybeeSystemRead(capture, samples, 8, &failure) == 0);
    assert(failure == expectedFailure);
    char detail[1024];
    daybeeSystemFailureDetail(capture, detail, sizeof(detail));
    assert(strstr(detail, "System audio stream interrupted"));
    assert(strstr(detail, SCStreamErrorDomain.UTF8String));
    assert(strstr(detail, "capture connection was invalidated"));
    assert(strstr(detail, "Native audio route failed"));
    assert(strstr(detail, "-50"));
    char number[32];
    snprintf(number, sizeof(number), "%ld", (long)code);
    assert(strstr(detail, number));
    daybeeSystemDestroy(capture);
}

// Does not create a process tap or open a device. Only the retained failure text is checked.
static void verifyTapStatus(NSString *detail) {
    DayBeeSystemTap *capture = [[DayBeeSystemTap alloc] init];
    daybeeVoiceBufferInit(&capture->buffer);
    atomic_init(&capture->stopping, false);
    @synchronized(capture) { capture->failureDetail = [detail retain]; }
    daybeeVoiceBufferFail(&capture->buffer, DAYBEE_VOICE_DISCONNECTED);
    float samples[8];
    int failure = 0;
    assert(daybeeSystemTapRead(capture, samples, 8, &failure) == 0);
    assert(failure == DAYBEE_VOICE_DISCONNECTED);
    char reported[1024];
    daybeeSystemTapFailureDetail(capture, reported, sizeof(reported));
    assert(strstr(reported, "系统音频录制权限"));
    assert(strstr(reported, "OSStatus -2003329396"));
    daybeeSystemTapStop(capture);
    daybeeSystemTapStop(capture);
    daybeeSystemTapDestroy(capture);
}

int main(void) {
    @autoreleasepool {
        verifyStreamError(SCStreamErrorFailedApplicationConnectionInterrupted, DAYBEE_VOICE_DISCONNECTED);
        verifyStreamError(SCStreamErrorUserStopped, DAYBEE_VOICE_USER_STOPPED);
        verifyTapStatus(@"系统音频录制权限未授予，System audio recording permission was denied (OSStatus -2003329396)");
    }
    return 0;
}
