#import <AVFAudio/AVFAudio.h>
#import <AudioToolbox/AudioToolbox.h>
#import <CoreAudio/CoreAudio.h>
#import "capture_voice_darwin.h"
#import "voice_buffer.h"
#import "voice_activity_darwin.h"

@interface DayBeeVoiceCapture : NSObject {
@public
    DayBeeVoiceBuffer buffer;
    _Atomic(bool) stopping;
    AVAudioEngine *engine;
    AVAudioConverter *converter;
    AVAudioPCMBuffer *converted;
    id observer;
    BOOL tapped;
    DayBeeVoiceActivity activity;
}
- (void)stop;
@end

@implementation DayBeeVoiceCapture
- (void)stop {
    atomic_store(&stopping, true);
    if (observer) {
        [[NSNotificationCenter defaultCenter] removeObserver:observer];
        [observer release];
        observer = nil;
    }
    [engine stop];
    if (tapped) {
        [engine.inputNode removeTapOnBus:0];
        tapped = NO;
    }
}
- (void)dealloc {
    [self stop];
    daybeeVoiceActivityClose(&activity);
    [converted release];
    [converter release];
    [engine release];
    [super dealloc];
}
@end

static BOOL selectInput(AVAudioInputNode *input, const char *uid, NSString **failure) {
    if (!uid[0]) return YES; // Follow the system default input.
    CFStringRef deviceUID = CFStringCreateWithCString(NULL, uid, kCFStringEncodingUTF8);
    if (!deviceUID) { *failure = @"Invalid microphone device UID"; return NO; }
    AudioDeviceID device = kAudioObjectUnknown;
    AudioObjectPropertyAddress address = {kAudioHardwarePropertyTranslateUIDToDevice,
        kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    UInt32 size = sizeof(device);
    OSStatus status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address,
        sizeof(deviceUID), &deviceUID, &size, &device);
    CFRelease(deviceUID);
    if (status == noErr && device != kAudioObjectUnknown) {
        status = AudioUnitSetProperty(input.audioUnit, kAudioOutputUnitProperty_CurrentDevice,
            kAudioUnitScope_Global, 1, &device, sizeof(device));
    } else if (status == noErr) {
        status = kAudioHardwareBadDeviceError;
    }
    if (status == noErr) return YES;
    *failure = [NSString stringWithFormat:@"Could not select microphone (Core Audio %d)", (int)status];
    return NO;
}

static BOOL prepareCapture(DayBeeVoiceCapture *capture, const char *uid, BOOL detectSpeech, NSString **failure) {
    if (detectSpeech) daybeeVoiceActivityOpen(&capture->activity, uid);
    capture->engine = [[AVAudioEngine alloc] init];
    AVAudioInputNode *input = capture->engine.inputNode;
    NSError *error = nil;
    if (![input setVoiceProcessingEnabled:YES error:&error] || !input.isVoiceProcessingEnabled) {
        *failure = [@"Enable voice processing: " stringByAppendingString:error.localizedDescription ?: @"unavailable"];
        return NO;
    }
    if (!selectInput(input, uid, failure)) return NO;
    // Do not amplify the room or turn the video down to simulate cancellation.
    input.voiceProcessingAGCEnabled = NO;
    input.voiceProcessingBypassed = NO;
    input.voiceProcessingInputMuted = NO;
    if (@available(macOS 14.0, *)) {
        input.voiceProcessingOtherAudioDuckingConfiguration =
            (AVAudioVoiceProcessingOtherAudioDuckingConfiguration){NO, AVAudioVoiceProcessingOtherAudioDuckingLevelMin};
    }
    AVAudioFormat *hardware = [input inputFormatForBus:0];
    if (hardware.sampleRate <= 0 || hardware.channelCount == 0) {
        *failure = @"Microphone has no usable audio format";
        return NO;
    }
    // The macOS voice-processing aggregate also exposes render-reference
    // channels (nine on some Macs). Request one processed microphone channel
    // from VoiceProcessingIO; downmixing the aggregate can select silence.
    AVAudioFormat *source = [[[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32
        sampleRate:hardware.sampleRate channels:1 interleaved:NO] autorelease];
    // Pull the microphone through a complete graph, with matching client-side
    // input/output formats required by VoiceProcessingIO. The tap precedes the
    // muted mixer, so captured speech is preserved without audible monitoring.
    [capture->engine connect:input to:capture->engine.mainMixerNode format:source];
    capture->engine.mainMixerNode.outputVolume = 0;
    [capture->engine connect:capture->engine.mainMixerNode to:capture->engine.outputNode format:source];
    AVAudioFormat *target = [[[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32
        sampleRate:16000 channels:1 interleaved:NO] autorelease];
    capture->converter = [[AVAudioConverter alloc] initFromFormat:source toFormat:target];
    capture->converted = [[AVAudioPCMBuffer alloc] initWithPCMFormat:target frameCapacity:4096];
    if (!capture->converter || !capture->converted) {
        *failure = @"Could not initialize microphone audio conversion";
        return NO;
    }
    [input installTapOnBus:0 bufferSize:1024 format:source block:^(AVAudioPCMBuffer *pcm, AVAudioTime *time) {
        if (atomic_load(&capture->stopping)) return;
        __block BOOL supplied = NO;
        // Drain the converter in bounded chunks; never assume the tap's actual
        // frame count equals the requested buffer size or reset SRC per buffer.
        for (;;) {
            NSError *conversionError = nil;
            capture->converted.frameLength = 0;
            AVAudioConverterOutputStatus status = [capture->converter convertToBuffer:capture->converted
                error:&conversionError withInputFromBlock:^AVAudioBuffer *(AVAudioPacketCount count, AVAudioConverterInputStatus *inputStatus) {
                    *inputStatus = supplied ? AVAudioConverterInputStatus_NoDataNow : AVAudioConverterInputStatus_HaveData;
                    if (supplied) return nil;
                    supplied = YES;
                    return pcm;
                }];
            if (status == AVAudioConverterOutputStatus_Error) {
                daybeeVoiceBufferFail(&capture->buffer, DAYBEE_VOICE_CONVERSION_FAILED);
                return;
            }
            if (capture->converted.frameLength) {
                daybeeVoiceBufferWrite(&capture->buffer, capture->converted.floatChannelData[0], capture->converted.frameLength);
            }
            if (status != AVAudioConverterOutputStatus_HaveData || atomic_load(&capture->buffer.failure)) return;
        }
    }];
    capture->tapped = YES;
    [capture->engine prepare];
    if (![capture->engine startAndReturnError:&error]) {
        *failure = [NSString stringWithFormat:@"Start audio engine (%@ -> %@): %@", source,
            [capture->engine.outputNode inputFormatForBus:0], error.localizedDescription ?: @"failed"];
        return NO;
    }
    // AVAudioEngine stops when its device configuration changes. Let the
    // recorder's existing disconnect path reopen processing on the new route.
    capture->observer = [[[NSNotificationCenter defaultCenter]
        addObserverForName:AVAudioEngineConfigurationChangeNotification object:capture->engine queue:nil
        usingBlock:^(NSNotification *notification) {
            if (!atomic_load(&capture->stopping)) daybeeVoiceBufferFail(&capture->buffer, DAYBEE_VOICE_DISCONNECTED);
        }] retain];
    return YES;
}

void *daybeeVoiceOpen(const char *uid, int detectSpeech, char *error, size_t errorSize) {
    @autoreleasepool {
        DayBeeVoiceCapture *capture = [[DayBeeVoiceCapture alloc] init];
        daybeeVoiceBufferInit(&capture->buffer);
        atomic_init(&capture->stopping, false);
        NSString *failure = nil;
        @try {
            if (prepareCapture(capture, uid, detectSpeech != 0, &failure)) return capture;
        } @catch (NSException *exception) {
            failure = exception.reason ?: @"Voice processing initialization failed";
        }
        snprintf(error, errorSize, "%s", (failure ?: @"Voice processing initialization failed").UTF8String);
        [capture stop];
        [capture release];
        return NULL;
    }
}

uint32_t daybeeVoiceRead(void *pointer, float *samples, uint32_t capacity, int *failure) {
    DayBeeVoiceCapture *capture = (DayBeeVoiceCapture *)pointer;
    *failure = atomic_load(&capture->buffer.failure);
    return daybeeVoiceBufferRead(&capture->buffer, samples, capacity);
}

int daybeeVoiceSpeechActivity(void *pointer) {
    return daybeeVoiceActivityRead(&((DayBeeVoiceCapture *)pointer)->activity);
}

void daybeeVoiceStop(void *pointer) {
    @autoreleasepool { [(DayBeeVoiceCapture *)pointer stop]; }
}

void daybeeVoiceDestroy(void *pointer) {
    @autoreleasepool { [(DayBeeVoiceCapture *)pointer release]; }
}
