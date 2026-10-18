#ifndef DAYBEE_SYSTEM_CAPTURE_H
#define DAYBEE_SYSTEM_CAPTURE_H
#include <stdint.h>
#include <stddef.h>
void *daybeeSystemOpen(char *error, size_t errorSize);
uint32_t daybeeSystemRead(void *capture, float *samples, uint32_t capacity, int *failure);
void daybeeSystemFailureDetail(void *capture, char *error, size_t errorSize);
void daybeeSystemStop(void *capture);
void daybeeSystemDestroy(void *capture);
#endif
