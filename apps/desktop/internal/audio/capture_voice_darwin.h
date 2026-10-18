#ifndef DAYBEE_VOICE_CAPTURE_H
#define DAYBEE_VOICE_CAPTURE_H
#include <stdint.h>
#include <stddef.h>

void *daybeeVoiceOpen(const char *uid, int detectSpeech, char *error, size_t errorSize);
uint32_t daybeeVoiceRead(void *capture, float *samples, uint32_t capacity, int *failure);
int daybeeVoiceSpeechActivity(void *capture);
void daybeeVoiceStop(void *capture);
void daybeeVoiceDestroy(void *capture);
#endif
