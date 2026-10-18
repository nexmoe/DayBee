#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreMedia/CoreMedia.h>
#import <AudioToolbox/AudioToolbox.h>
#import "capture_system_darwin.h"
#import "capture_system_tap_darwin.h"
#import "voice_buffer.h"

static NSString *systemErrorDescription(NSError *error) {
    if (!error) return @"";
    NSMutableString *description = [NSMutableString stringWithFormat:@"%@ (%@ %ld)",
        error.localizedDescription, error.domain, (long)error.code];
    if (error.localizedFailureReason.length) [description appendFormat:@". %@", error.localizedFailureReason];
    NSError *underlying = error.userInfo[NSUnderlyingErrorKey];
    if (underlying) [description appendFormat:@". Caused by: %@ (%@ %ld)",
        underlying.localizedDescription, underlying.domain, (long)underlying.code];
    return description;
}

// Only an audio output is registered. No screen frames are delivered or saved.
@interface DayBeeSystemCapture : NSObject <SCStreamOutput, SCStreamDelegate> {
@public
    DayBeeVoiceBuffer buffer;
    _Atomic(bool) stopping;
    SCStream *stream;
    dispatch_queue_t queue;
    BOOL attached;
    BOOL started;
    NSError *streamError;
}
- (void)stop;
@end

@implementation DayBeeSystemCapture
- (void)stream:(SCStream *)sender didStopWithError:(NSError *)error {
    if (!atomic_load(&stopping)) {
        @synchronized(self) {
            if (!streamError) streamError = [error retain];
        }
        int failure = [error.domain isEqualToString:SCStreamErrorDomain] && error.code == SCStreamErrorUserStopped
            ? DAYBEE_VOICE_USER_STOPPED : DAYBEE_VOICE_DISCONNECTED;
        daybeeVoiceBufferFail(&buffer, failure);
    }
}
- (void)stream:(SCStream *)sender didOutputSampleBuffer:(CMSampleBufferRef)sample ofType:(SCStreamOutputType)type {
    if (atomic_load(&stopping) || type != SCStreamOutputTypeAudio || !CMSampleBufferDataIsReady(sample)) return;
    const AudioStreamBasicDescription *format = CMAudioFormatDescriptionGetStreamBasicDescription(CMSampleBufferGetFormatDescription(sample));
    if (!format || format->mSampleRate != 16000 || format->mChannelsPerFrame != 1 ||
        format->mFormatID != kAudioFormatLinearPCM || !(format->mFormatFlags & kAudioFormatFlagIsFloat) || format->mBitsPerChannel != 32) {
        daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
        return;
    }
    AudioBufferList list;
    CMBlockBufferRef block = NULL;
    OSStatus status = CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(sample, NULL,
        &list, sizeof(list), kCFAllocatorDefault, kCFAllocatorDefault,
        kCMSampleBufferFlag_AudioBufferList_Assure16ByteAlignment, &block);
    if (status == noErr && list.mNumberBuffers == 1 && list.mBuffers[0].mData &&
        list.mBuffers[0].mDataByteSize == CMSampleBufferGetNumSamples(sample) * sizeof(float)) {
        daybeeVoiceBufferWrite(&buffer, list.mBuffers[0].mData, (uint32_t)CMSampleBufferGetNumSamples(sample));
    } else {
        daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_CONVERSION_FAILED);
    }
    if (block) CFRelease(block);
}
- (void)stop {
    if (atomic_exchange(&stopping, true)) return;
    if (started) {
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        [stream stopCaptureWithCompletionHandler:^(NSError *error) { dispatch_semaphore_signal(done); }];
        dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
        dispatch_release(done);
        started = NO;
    }
    if (attached) {
        [stream removeStreamOutput:self type:SCStreamOutputTypeAudio error:nil];
        attached = NO;
    }
    // Finish any native callbacks before the Go polling owner releases us.
    if (queue) dispatch_sync(queue, ^{});
}
- (void)dealloc {
    [self stop];
    [stream release];
    [streamError release];
    if (queue) dispatch_release(queue);
    [super dealloc];
}
@end

static BOOL prepareSystemCapture(DayBeeSystemCapture *capture, NSString **failure) {
    dispatch_semaphore_t contentReady = dispatch_semaphore_create(0);
    __block SCShareableContent *content = nil;
    __block NSError *contentError = nil;
    [SCShareableContent getShareableContentExcludingDesktopWindows:YES onScreenWindowsOnly:YES
        completionHandler:^(SCShareableContent *result, NSError *error) {
            content = [result retain];
            contentError = [error retain];
            dispatch_semaphore_signal(contentReady);
        }];
    dispatch_semaphore_wait(contentReady, DISPATCH_TIME_FOREVER);
    dispatch_release(contentReady);
    if (!content || content.displays.count == 0) {
        *failure = contentError ? systemErrorDescription(contentError) : @"No display is available for system audio capture";
        [contentError release];
        [content release];
        return NO;
    }
    [contentError release];
    SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:content.displays.firstObject excludingWindows:@[]];
    [content release];
    SCStreamConfiguration *configuration = [[SCStreamConfiguration alloc] init];
    configuration.capturesAudio = YES;
    configuration.excludesCurrentProcessAudio = YES;
    configuration.sampleRate = 16000;
    configuration.channelCount = 1;
    configuration.width = 2;
    configuration.height = 2;
    configuration.minimumFrameInterval = CMTimeMake(1, 1);
    configuration.showsCursor = NO;
    // The microphone belongs exclusively to the separate AEC capture path.
    if (@available(macOS 15.0, *)) configuration.captureMicrophone = NO;
    capture->queue = dispatch_queue_create("build.nexmoe.daybee.system-audio", DISPATCH_QUEUE_SERIAL);
    capture->stream = [[SCStream alloc] initWithFilter:filter configuration:configuration delegate:capture];
    [filter release];
    [configuration release];
    NSError *outputError = nil;
    if (![capture->stream addStreamOutput:capture type:SCStreamOutputTypeAudio sampleHandlerQueue:capture->queue error:&outputError]) {
        *failure = outputError ? systemErrorDescription(outputError) : @"Could not attach system audio output";
        return NO;
    }
    capture->attached = YES;
    dispatch_semaphore_t started = dispatch_semaphore_create(0);
    __block NSError *startError = nil;
    [capture->stream startCaptureWithCompletionHandler:^(NSError *error) {
        startError = [error retain];
        dispatch_semaphore_signal(started);
    }];
    dispatch_semaphore_wait(started, DISPATCH_TIME_FOREVER);
    dispatch_release(started);
    if (startError) {
        *failure = systemErrorDescription(startError);
        [startError release];
        return NO;
    }
    capture->started = YES;
    return YES;
}

void *daybeeSystemOpen(char *error, size_t errorSize) {
    // 14.2+ uses a Core Audio process tap (System Audio Recording only).
    // 13.3–14.1 keep ScreenCaptureKit, which still needs Screen Recording.
    if (@available(macOS 14.2, *)) return daybeeSystemTapOpen(error, errorSize);
    @autoreleasepool {
        DayBeeSystemCapture *capture = [[DayBeeSystemCapture alloc] init];
        daybeeVoiceBufferInit(&capture->buffer);
        atomic_init(&capture->stopping, false);
        NSString *failure = nil;
        @try {
            if (prepareSystemCapture(capture, &failure)) return capture;
        } @catch (NSException *exception) {
            failure = exception.reason;
        }
        snprintf(error, errorSize, "%s", (failure ?: @"System audio capture failed").UTF8String);
        [capture stop];
        [capture release];
        return NULL;
    }
}
uint32_t daybeeSystemRead(void *pointer, float *samples, uint32_t capacity, int *failure) {
    DayBeeSystemCapture *capture = (DayBeeSystemCapture *)pointer;
    *failure = atomic_load(&capture->buffer.failure);
    return daybeeVoiceBufferRead(&capture->buffer, samples, capacity);
}
void daybeeSystemFailureDetail(void *pointer, char *error, size_t errorSize) {
    @autoreleasepool {
        DayBeeSystemCapture *capture = (DayBeeSystemCapture *)pointer;
        @synchronized(capture) {
            snprintf(error, errorSize, "%s", systemErrorDescription(capture->streamError).UTF8String);
        }
    }
}
void daybeeSystemStop(void *pointer) { @autoreleasepool { [(DayBeeSystemCapture *)pointer stop]; } }
void daybeeSystemDestroy(void *pointer) { @autoreleasepool { [(DayBeeSystemCapture *)pointer release]; } }
