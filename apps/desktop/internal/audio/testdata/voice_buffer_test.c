#include "../voice_buffer.h"
#include <assert.h>
#include <pthread.h>
#include <sched.h>
#include <stdlib.h>

static DayBeeVoiceBuffer buffer;
enum { TOTAL = 1000000 };

static void *produce(void *unused) {
    for (uint32_t i = 0; i < TOTAL; i++) {
        // Keep this test within capacity; overflow is tested separately.
        while (atomic_load(&buffer.written) - atomic_load(&buffer.read) >= DAYBEE_VOICE_CAPACITY - 1) sched_yield();
        float value = (float)i;
        daybeeVoiceBufferWrite(&buffer, &value, 1);
    }
    return NULL;
}

int main(void) {
    daybeeVoiceBufferInit(&buffer);
    pthread_t producer;
    assert(pthread_create(&producer, NULL, produce, NULL) == 0);
    uint32_t received = 0;
    float output[511];
    while (received < TOTAL) {
        uint32_t count = daybeeVoiceBufferRead(&buffer, output, 511);
        for (uint32_t i = 0; i < count; i++) assert(output[i] == (float)received++);
        assert(atomic_load(&buffer.failure) == 0);
    }
    pthread_join(producer, NULL);
    assert(daybeeVoiceBufferRead(&buffer, output, 511) == 0);

    // Overflow must neither overwrite unread speech nor clear an earlier error.
    daybeeVoiceBufferInit(&buffer);
    float *full = malloc(DAYBEE_VOICE_CAPACITY * sizeof(float));
    assert(full);
    for (uint32_t i = 0; i < DAYBEE_VOICE_CAPACITY; i++) full[i] = (float)i;
    daybeeVoiceBufferWrite(&buffer, full, DAYBEE_VOICE_CAPACITY);
    daybeeVoiceBufferWrite(&buffer, full, 1);
    assert(atomic_load(&buffer.failure) == DAYBEE_VOICE_OVERFLOW);
    daybeeVoiceBufferFail(&buffer, DAYBEE_VOICE_DISCONNECTED);
    assert(atomic_load(&buffer.failure) == DAYBEE_VOICE_OVERFLOW);
    assert(daybeeVoiceBufferRead(&buffer, full, DAYBEE_VOICE_CAPACITY) == DAYBEE_VOICE_CAPACITY);
    for (uint32_t i = 0; i < DAYBEE_VOICE_CAPACITY; i++) assert(full[i] == (float)i);
    free(full);
    return 0;
}
