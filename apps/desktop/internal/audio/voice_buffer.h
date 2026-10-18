#ifndef DAYBEE_VOICE_BUFFER_H
#define DAYBEE_VOICE_BUFFER_H

#include <stdatomic.h>
#include <stdint.h>
#include <string.h>

enum { DAYBEE_VOICE_CAPACITY = 32768, DAYBEE_VOICE_DISCONNECTED = 1,
       DAYBEE_VOICE_OVERFLOW = 2, DAYBEE_VOICE_CONVERSION_FAILED = 3, DAYBEE_VOICE_USER_STOPPED = 4 };

// One native producer, one Go polling consumer. Never allocate, wait, or call
// Go from the audio callback. A full buffer is an error, not dropped speech.
typedef struct {
    float samples[DAYBEE_VOICE_CAPACITY];
    _Atomic(uint64_t) read;
    _Atomic(uint64_t) written;
    _Atomic(int) failure;
} DayBeeVoiceBuffer;

static inline void daybeeVoiceBufferInit(DayBeeVoiceBuffer *buffer) {
    atomic_init(&buffer->read, 0);
    atomic_init(&buffer->written, 0);
    atomic_init(&buffer->failure, 0);
}

static inline void daybeeVoiceBufferFail(DayBeeVoiceBuffer *buffer, int failure) {
    int expected = 0;
    atomic_compare_exchange_strong(&buffer->failure, &expected, failure);
}

static inline void daybeeVoiceBufferWrite(DayBeeVoiceBuffer *buffer, const float *samples, uint32_t count) {
    if (atomic_load(&buffer->failure)) return;
    uint64_t written = atomic_load_explicit(&buffer->written, memory_order_relaxed);
    uint64_t read = atomic_load_explicit(&buffer->read, memory_order_acquire);
    if (count > DAYBEE_VOICE_CAPACITY - (written - read)) {
        daybeeVoiceBufferFail(buffer, DAYBEE_VOICE_OVERFLOW);
        return;
    }
    uint32_t offset = written % DAYBEE_VOICE_CAPACITY;
    uint32_t first = count < DAYBEE_VOICE_CAPACITY - offset ? count : DAYBEE_VOICE_CAPACITY - offset;
    memcpy(buffer->samples + offset, samples, first * sizeof(float));
    memcpy(buffer->samples, samples + first, (count - first) * sizeof(float));
    atomic_store_explicit(&buffer->written, written + count, memory_order_release);
}

static inline uint32_t daybeeVoiceBufferRead(DayBeeVoiceBuffer *buffer, float *samples, uint32_t capacity) {
    uint64_t read = atomic_load_explicit(&buffer->read, memory_order_relaxed);
    uint64_t written = atomic_load_explicit(&buffer->written, memory_order_acquire);
    uint32_t count = written - read < capacity ? (uint32_t)(written - read) : capacity;
    uint32_t offset = read % DAYBEE_VOICE_CAPACITY;
    uint32_t first = count < DAYBEE_VOICE_CAPACITY - offset ? count : DAYBEE_VOICE_CAPACITY - offset;
    memcpy(samples, buffer->samples + offset, first * sizeof(float));
    memcpy(samples + first, buffer->samples, (count - first) * sizeof(float));
    atomic_store_explicit(&buffer->read, read + count, memory_order_release);
    return count;
}
#endif
