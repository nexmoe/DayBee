#ifndef DAYBEE_SYSTEM_TAP_H
#define DAYBEE_SYSTEM_TAP_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif

// Core Audio process tap, macOS 14.2+. Same C surface as the ScreenCaptureKit path.
void *daybeeSystemTapOpen(char *error, size_t errorSize);
uint32_t daybeeSystemTapRead(void *capture, float *samples, uint32_t capacity, int *failure);
void daybeeSystemTapFailureDetail(void *capture, char *error, size_t errorSize);
void daybeeSystemTapStop(void *capture);
void daybeeSystemTapDestroy(void *capture);
int daybeeSystemTapAtLeast14_2(void);

#ifdef __cplusplus
}
#endif
#endif
