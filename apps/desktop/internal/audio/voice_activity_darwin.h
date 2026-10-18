#ifndef DAYBEE_VOICE_ACTIVITY_H
#define DAYBEE_VOICE_ACTIVITY_H

#include <CoreAudio/CoreAudio.h>
#include <stdbool.h>

typedef struct {
    AudioDeviceID device;
    AudioDeviceIOProcID io;
    UInt32 previous;
    bool enabled;
} DayBeeVoiceActivity;

void daybeeVoiceActivityOpen(DayBeeVoiceActivity *activity, const char *uid);
int daybeeVoiceActivityRead(DayBeeVoiceActivity *activity);
void daybeeVoiceActivityClose(DayBeeVoiceActivity *activity);

#endif
