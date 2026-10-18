#import <Foundation/Foundation.h>
#include <string.h>
#import "voice_activity_darwin.h"

// Core Audio's detector analyzes echo-cancelled input independently of Silero.
// Keep a physical-device input active: AVAudioEngine's private voice-processing
// aggregate does not reliably activate this detector for the physical device.
static OSStatus activityInput(AudioObjectID device, const AudioTimeStamp *now,
    const AudioBufferList *input, const AudioTimeStamp *inputTime,
    AudioBufferList *output, const AudioTimeStamp *outputTime, void *context) {
    // Duplex USB devices may also supply output buffers to an input IOProc.
    // This client never renders sound; other clients keep their normal mix.
    if (output) {
        for (UInt32 i = 0; i < output->mNumberBuffers; i++) {
            if (output->mBuffers[i].mData) {
                memset(output->mBuffers[i].mData, 0, output->mBuffers[i].mDataByteSize);
            }
        }
    }
    return noErr;
}

void daybeeVoiceActivityClose(DayBeeVoiceActivity *activity) {
    if (activity->io) {
        AudioDeviceStop(activity->device, activity->io);
        AudioDeviceDestroyIOProcID(activity->device, activity->io);
        activity->io = NULL;
    }
    if (activity->enabled) {
        AudioObjectPropertyAddress address = {kAudioDevicePropertyVoiceActivityDetectionEnable,
            kAudioDevicePropertyScopeInput, kAudioObjectPropertyElementMain};
        AudioObjectSetPropertyData(activity->device, &address, 0, NULL,
            sizeof(activity->previous), &activity->previous);
        activity->enabled = false;
    }
}

void daybeeVoiceActivityOpen(DayBeeVoiceActivity *activity, const char *uid) {
    if (@available(macOS 14.0, *)) {
        AudioObjectPropertyAddress address = {kAudioHardwarePropertyDefaultInputDevice,
            kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
        UInt32 size = sizeof(activity->device);
        OSStatus status;
        if (uid[0]) {
            CFStringRef deviceUID = CFStringCreateWithCString(NULL, uid, kCFStringEncodingUTF8);
            if (!deviceUID) return;
            address.mSelector = kAudioHardwarePropertyTranslateUIDToDevice;
            status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address,
                sizeof(deviceUID), &deviceUID, &size, &activity->device);
            CFRelease(deviceUID);
        } else {
            status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address,
                0, NULL, &size, &activity->device);
        }
        if (status != noErr || activity->device == kAudioObjectUnknown) return;
        address = (AudioObjectPropertyAddress){kAudioDevicePropertyVoiceActivityDetectionEnable,
            kAudioDevicePropertyScopeInput, kAudioObjectPropertyElementMain};
        size = sizeof(activity->previous);
        if (AudioObjectGetPropertyData(activity->device, &address, 0, NULL,
            &size, &activity->previous) != noErr) return;
        UInt32 enabled = 1;
        if (AudioObjectSetPropertyData(activity->device, &address, 0, NULL,
            sizeof(enabled), &enabled) != noErr) return;
        activity->enabled = true;
        status = AudioDeviceCreateIOProcID(activity->device, activityInput, NULL, &activity->io);
        if (status == noErr) status = AudioDeviceStart(activity->device, activity->io);
        if (status != noErr) daybeeVoiceActivityClose(activity);
    }
}

int daybeeVoiceActivityRead(DayBeeVoiceActivity *activity) {
    if (!activity->enabled || !activity->io) return -1;
    AudioObjectPropertyAddress address = {kAudioDevicePropertyVoiceActivityDetectionState,
        kAudioDevicePropertyScopeInput, kAudioObjectPropertyElementMain};
    UInt32 speech = 0, size = sizeof(speech);
    if (AudioObjectGetPropertyData(activity->device, &address, 0, NULL, &size, &speech) != noErr) return -1;
    return speech ? 1 : 0;
}
