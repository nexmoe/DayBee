#import <CoreAudio/AudioHardware.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <AudioToolbox/AudioToolbox.h>
#import <Foundation/Foundation.h>
#import "capture_system_tap_darwin.h"
#import "capture_system_tap_objc.h"

// Global process tap on macOS 14.2+. Needs System Audio Recording
// (kTCCServiceAudioCapture) only; Screen Recording is not requested.
// 14.x does not resample the tap, so AudioConverter produces 16 kHz mono float32.

static NSString *tapStatus(NSString *action, OSStatus status) {
    return [NSString stringWithFormat:@"%@ (OSStatus %d)", action, (int)status];
}

static AudioObjectPropertyAddress tapAddress(AudioObjectPropertySelector selector, AudioObjectPropertyScope scope) {
    return (AudioObjectPropertyAddress){selector, scope, kAudioObjectPropertyElementMain};
}

static BOOL readProperty(AudioObjectID object, AudioObjectPropertySelector selector,
    AudioObjectPropertyScope scope, UInt32 *size, void *value) {
    AudioObjectPropertyAddress address = tapAddress(selector, scope);
    return AudioObjectGetPropertyData(object, &address, 0, NULL, size, value) == noErr;
}

@implementation DayBeeSystemTap

static OSStatus tapInput(AudioConverterRef converter, UInt32 *count, AudioBufferList *data,
    AudioStreamPacketDescription **packets, void *user) {
    AudioBufferList *source = (AudioBufferList *)user;
    if (*count == 0 || source->mBuffers[0].mDataByteSize == 0) {
        *count = 0;
        return noErr;
    }
    *count = 1;
    data->mNumberBuffers = source->mNumberBuffers;
    for (UInt32 i = 0; i < source->mNumberBuffers; i++) data->mBuffers[i] = source->mBuffers[i];
    source->mBuffers[0].mDataByteSize = 0;
    return noErr;
}

- (void)receive:(const AudioBufferList *)input {
    if (atomic_load(&stopping) || atomic_load(&buffer.failure) || !converter || !input || bytesPerFrame == 0) return;
    if (input->mNumberBuffers == 0 || input->mBuffers[0].mDataByteSize == 0) return;
    UInt32 frames = input->mBuffers[0].mDataByteSize / bytesPerFrame;
    if (frames == 0) return;
    UInt32 buffers = input->mNumberBuffers;
    if (buffers == 0 || buffers > 32) {
        daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
        return;
    }
    UInt32 bytes = 0;
    for (UInt32 i = 0; i < buffers; i++) {
        if (!input->mBuffers[i].mData) {
            daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
            return;
        }
        bytes += input->mBuffers[i].mDataByteSize;
    }
    // Header size follows the channel count; a smaller payload must not shrink it.
    size_t header = sizeof(AudioBufferList) + (buffers - 1) * sizeof(AudioBuffer);
    if (sourceBuffers < buffers || sourceBytes < bytes) {
        float *grown = realloc(sourceList, header + bytes);
        if (!grown) {
            daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
            return;
        }
        sourceList = (AudioBufferList *)grown;
        sourceBuffers = buffers;
        sourceBytes = bytes;
    }
    sourceList->mNumberBuffers = buffers;
    uint8_t *cursor = (uint8_t *)sourceList + sizeof(AudioBufferList) + (buffers - 1) * sizeof(AudioBuffer);
    for (UInt32 i = 0; i < buffers; i++) {
        sourceList->mBuffers[i] = input->mBuffers[i];
        memcpy(cursor, input->mBuffers[i].mData, input->mBuffers[i].mDataByteSize);
        sourceList->mBuffers[i].mData = cursor;
        cursor += input->mBuffers[i].mDataByteSize;
    }
    uint32_t need = frames * 4 + 64;
    if (convertedCapacity < need) {
        float *grown = realloc(converted, need * sizeof(float));
        if (!grown) {
            daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
            return;
        }
        converted = grown;
        convertedCapacity = need;
    }
    AudioBufferList output = {1, {{1, (UInt32)(convertedCapacity * sizeof(float)), converted}}};
    UInt32 packets = convertedCapacity;
    OSStatus status = AudioConverterFillComplexBuffer(converter, tapInput, sourceList, &packets, &output, NULL);
    if (status != noErr) {
        @synchronized(self) {
            if (!failureDetail) failureDetail = [tapStatus(@"System audio conversion failed", status) retain];
        }
        daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
        return;
    }
    if (packets) daybeeVoiceBufferWrite(&buffer, converted, packets);
}

- (void)reportDisconnected:(NSString *)detail {
    if (atomic_load(&stopping)) return;
    @synchronized(self) {
        if (!failureDetail) failureDetail = [detail retain];
    }
    daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_DISCONNECTED);
}

- (void)stop {
    if (atomic_exchange(&stopping, true)) return;
    AudioObjectPropertyAddress output = tapAddress(kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal);
    AudioObjectPropertyAddress alive = tapAddress(kAudioDevicePropertyDeviceIsAlive, kAudioObjectPropertyScopeGlobal);
    if (listening && deviceListener) {
        AudioObjectRemovePropertyListenerBlock(kAudioObjectSystemObject, &output, queue, deviceListener);
        if (aggregateID) AudioObjectRemovePropertyListenerBlock(aggregateID, &alive, queue, deviceListener);
        listening = NO;
    }
    if (aggregateID && ioProc) AudioDeviceStop(aggregateID, ioProc);
    if (aggregateID && ioProc) {
        AudioDeviceDestroyIOProcID(aggregateID, ioProc);
        ioProc = NULL;
    }
    if (aggregateID) {
        AudioHardwareDestroyAggregateDevice(aggregateID);
        aggregateID = 0;
    }
    if (tapID) {
        if (@available(macOS 14.2, *)) AudioHardwareDestroyProcessTap(tapID);
        tapID = 0;
    }
    if (converter) {
        AudioConverterDispose(converter);
        converter = NULL;
    }
    // Finish any IO block before the Go polling owner releases us.
    if (queue) dispatch_sync(queue, ^{});
}

- (void)dealloc {
    [self stop];
    if (deviceListener) Block_release(deviceListener);
    [failureDetail release];
    free(sourceList);
    free(converted);
    if (queue) dispatch_release(queue);
    [super dealloc];
}
@end

static BOOL translateSelf(AudioObjectID *process, NSString **failure) {
    pid_t pid = getpid();
    AudioObjectPropertyAddress address = tapAddress(kAudioHardwarePropertyTranslatePIDToProcessObject, kAudioObjectPropertyScopeGlobal);
    UInt32 size = sizeof(*process);
    OSStatus status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address, sizeof(pid), &pid, &size, process);
    if (status != noErr || *process == kAudioObjectUnknown) {
        *failure = tapStatus(@"Could not resolve this process for system audio", status == noErr ? kAudioHardwareBadDeviceError : status);
        return NO;
    }
    return YES;
}

// A fresh tap can answer '!obj' until the HAL publishes it. Retry briefly.
static BOOL readTapFormat(AudioObjectID tap, AudioStreamBasicDescription *format, NSString **failure) {
    for (int attempt = 0; attempt < 8; attempt++) {
        UInt32 size = sizeof(*format);
        AudioObjectPropertyAddress address = tapAddress(kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal);
        OSStatus status = AudioObjectGetPropertyData(tap, &address, 0, NULL, &size, format);
        if (status == noErr && format->mSampleRate > 0 && format->mChannelsPerFrame > 0 &&
            format->mFormatID == kAudioFormatLinearPCM) return YES;
        if (status != noErr && status != kAudioHardwareBadObjectError) {
            *failure = tapStatus(@"Could not read the system audio tap format", status);
            return NO;
        }
        usleep(20000);
    }
    *failure = @"Could not read the system audio tap format (OSStatus !obj)";
    return NO;
}

static BOOL readTapUID(AudioObjectID tap, CFStringRef *uid, NSString **failure) {
    UInt32 size = sizeof(*uid);
    AudioObjectPropertyAddress address = tapAddress(kAudioTapPropertyUID, kAudioObjectPropertyScopeGlobal);
    OSStatus status = AudioObjectGetPropertyData(tap, &address, 0, NULL, &size, uid);
    if (status == noErr && *uid) return YES;
    *failure = tapStatus(@"Could not read the system audio tap UID", status);
    return NO;
}

// Chromium probes permission by writing the tap description back. There is no public preflight.
static BOOL probeTapPermission(AudioObjectID tap, NSString **failure) {
    CATapDescription *description = nil;
    UInt32 size = sizeof(description);
    AudioObjectPropertyAddress address = tapAddress(kAudioTapPropertyDescription, kAudioObjectPropertyScopeGlobal);
    OSStatus status = AudioObjectGetPropertyData(tap, &address, 0, NULL, &size, &description);
    if (status == noErr) {
        status = AudioObjectSetPropertyData(tap, &address, 0, NULL, size, &description);
    }
    if (status == noErr) return YES;
    *failure = [NSString stringWithFormat:
        @"系统音频录制权限未授予，System audio recording permission was denied (OSStatus %d)", (int)status];
    return NO;
}

static BOOL makeAggregate(DayBeeSystemTap *capture, CFStringRef tapUID, NSString **failure) {
    NSString *uid = [[NSUUID UUID] UUIDString];
    NSDictionary *description = @{
        @kAudioAggregateDeviceNameKey: @"DayBee System Audio",
        @kAudioAggregateDeviceUIDKey: uid,
        @kAudioAggregateDeviceIsPrivateKey: @YES,
        @kAudioAggregateDeviceTapAutoStartKey: @NO,
        @kAudioAggregateDeviceTapListKey: @[@{
            @kAudioSubTapUIDKey: (__bridge NSString *)tapUID,
            @kAudioSubTapDriftCompensationKey: @YES
        }]
    };
    OSStatus status = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)description, &capture->aggregateID);
    if (status == noErr && capture->aggregateID != kAudioObjectUnknown) return YES;
    capture->aggregateID = 0;
    *failure = tapStatus(@"Could not create the private system audio aggregate", status);
    return NO;
}

static BOOL makeConverter(DayBeeSystemTap *capture, const AudioStreamBasicDescription *source, NSString **failure) {
    AudioStreamBasicDescription target = {0};
    target.mSampleRate = 16000;
    target.mFormatID = kAudioFormatLinearPCM;
    target.mFormatFlags = kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked;
    target.mBitsPerChannel = 32;
    target.mChannelsPerFrame = 1;
    target.mBytesPerFrame = 4;
    target.mFramesPerPacket = 1;
    target.mBytesPerPacket = 4;
    OSStatus status = AudioConverterNew(source, &target, &capture->converter);
    if (status != noErr || !capture->converter) {
        *failure = tapStatus(@"Could not convert system audio to 16 kHz mono", status);
        return NO;
    }
    return YES;
}

static BOOL watchRoute(DayBeeSystemTap *capture, NSString **failure) {
    capture->deviceListener = Block_copy(^(UInt32 count, const AudioObjectPropertyAddress *addresses) {
        for (UInt32 i = 0; i < count; i++) {
            if (addresses[i].mSelector == kAudioHardwarePropertyDefaultOutputDevice) {
                [capture reportDisconnected:@"System audio output device changed"];
                return;
            }
        }
        UInt32 alive = 0, size = sizeof(alive);
        if (!readProperty(capture->aggregateID, kAudioDevicePropertyDeviceIsAlive,
                kAudioObjectPropertyScopeGlobal, &size, &alive) || !alive) {
            [capture reportDisconnected:@"System audio aggregate device is no longer alive"];
        }
    });
    AudioObjectPropertyAddress output = tapAddress(kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal);
    AudioObjectPropertyAddress alive = tapAddress(kAudioDevicePropertyDeviceIsAlive, kAudioObjectPropertyScopeGlobal);
    OSStatus status = AudioObjectAddPropertyListenerBlock(kAudioObjectSystemObject, &output, capture->queue, capture->deviceListener);
    if (status == noErr) {
        status = AudioObjectAddPropertyListenerBlock(capture->aggregateID, &alive, capture->queue, capture->deviceListener);
    }
    if (status == noErr) {
        capture->listening = YES;
        return YES;
    }
    *failure = tapStatus(@"Could not watch the system audio route", status);
    return NO;
}

static BOOL prepareTap(DayBeeSystemTap *capture, NSString **failure) {
    AudioObjectID process = kAudioObjectUnknown;
    if (!translateSelf(&process, failure)) return NO;
    CATapDescription *description = [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:@[@(process)]];
    description.name = @"DayBee System Audio";
    description.muteBehavior = CATapUnmuted;
    description.privateTap = YES;
    OSStatus status = noErr;
    if (@available(macOS 14.2, *)) {
        status = AudioHardwareCreateProcessTap(description, &capture->tapID);
    } else {
        status = kAudioHardwareUnsupportedOperationError;
    }
    [description release];
    if (status != noErr || capture->tapID == kAudioObjectUnknown) {
        capture->tapID = 0;
        *failure = tapStatus(@"Could not create the system audio process tap", status);
        return NO;
    }
    if (!probeTapPermission(capture->tapID, failure)) return NO;
    AudioStreamBasicDescription format = {0};
    CFStringRef tapUID = NULL;
    if (!readTapFormat(capture->tapID, &format, failure) || !readTapUID(capture->tapID, &tapUID, failure)) return NO;
    BOOL aggregate = makeAggregate(capture, tapUID, failure);
    CFRelease(tapUID);
    if (!aggregate || !makeConverter(capture, &format, failure)) return NO;
    UInt32 bytesPerFrame = format.mBytesPerFrame;
    if (bytesPerFrame == 0) bytesPerFrame = (format.mBitsPerChannel / 8) * MAX(format.mChannelsPerFrame, 1);
    if (bytesPerFrame == 0) {
        *failure = @"System audio tap format has no frame size";
        return NO;
    }
    capture->bytesPerFrame = bytesPerFrame;
    capture->queue = dispatch_queue_create("build.nexmoe.daybee.system-tap", DISPATCH_QUEUE_SERIAL);
    status = AudioDeviceCreateIOProcIDWithBlock(&capture->ioProc, capture->aggregateID, capture->queue,
        ^(const AudioTimeStamp *now, const AudioBufferList *input, const AudioTimeStamp *inputTime,
            AudioBufferList *output, const AudioTimeStamp *outputTime) {
            [capture receive:input];
        });
    if (status != noErr) {
        *failure = tapStatus(@"Could not attach the system audio callback", status);
        return NO;
    }
    if (!watchRoute(capture, failure)) return NO;
    status = AudioDeviceStart(capture->aggregateID, capture->ioProc);
    if (status != noErr) {
        *failure = tapStatus(@"Could not start system audio capture", status);
        return NO;
    }
    return YES;
}

void *daybeeSystemTapOpen(char *error, size_t errorSize) {
    @autoreleasepool {
        DayBeeSystemTap *capture = [[DayBeeSystemTap alloc] init];
        daybeeVoiceBufferInit(&capture->buffer);
        atomic_init(&capture->stopping, false);
        NSString *failure = nil;
        @try {
            if (prepareTap(capture, &failure)) return capture;
        } @catch (NSException *exception) {
            failure = exception.reason;
        }
        snprintf(error, errorSize, "%s", (failure ?: @"System audio capture failed").UTF8String);
        [capture stop];
        [capture release];
        return NULL;
    }
}

uint32_t daybeeSystemTapRead(void *pointer, float *samples, uint32_t capacity, int *failure) {
    DayBeeSystemTap *capture = (DayBeeSystemTap *)pointer;
    *failure = atomic_load(&capture->buffer.failure);
    return daybeeVoiceBufferRead(&capture->buffer, samples, capacity);
}

void daybeeSystemTapFailureDetail(void *pointer, char *error, size_t errorSize) {
    @autoreleasepool {
        DayBeeSystemTap *capture = (DayBeeSystemTap *)pointer;
        @synchronized(capture) {
            snprintf(error, errorSize, "%s", (capture->failureDetail ?: @"").UTF8String);
        }
    }
}

void daybeeSystemTapStop(void *pointer) { @autoreleasepool { [(DayBeeSystemTap *)pointer stop]; } }
void daybeeSystemTapDestroy(void *pointer) { @autoreleasepool { [(DayBeeSystemTap *)pointer release]; } }

int daybeeSystemTapAtLeast14_2(void) {
    return [[NSProcessInfo processInfo] isOperatingSystemAtLeastVersion:(NSOperatingSystemVersion){14, 2, 0}];
}
