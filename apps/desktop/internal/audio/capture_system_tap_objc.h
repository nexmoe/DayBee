#ifndef DAYBEE_SYSTEM_TAP_OBJC_H
#define DAYBEE_SYSTEM_TAP_OBJC_H
#ifdef __OBJC__
#import <AudioToolbox/AudioToolbox.h>
#import <CoreAudio/AudioHardware.h>
#import <Foundation/Foundation.h>
#import "voice_buffer.h"

// Visible to the native error test. Production code only calls the C functions.
@interface DayBeeSystemTap : NSObject {
@public
    DayBeeVoiceBuffer buffer;
    _Atomic(bool) stopping;
    AudioObjectID tapID;
    AudioObjectID aggregateID;
    AudioDeviceIOProcID ioProc;
    AudioConverterRef converter;
    dispatch_queue_t queue;
    AudioBufferList *sourceList;
    uint32_t sourceBytes;
    uint32_t sourceBuffers;
    float *converted;
    uint32_t convertedCapacity;
    NSString *failureDetail;
    AudioObjectPropertyListenerBlock deviceListener;
    UInt32 bytesPerFrame;
    BOOL listening;
}
- (void)stop;
@end
#endif
#endif
