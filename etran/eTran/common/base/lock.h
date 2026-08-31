#pragma once

#include <assert.h>
#include <stdint.h>
#include <stdbool.h>

#include <base/cpu.h>

typedef struct
{
    volatile int locked;
} spinlock_t;

#define SPINLOCK_INITIALIZER \
    {                        \
        .locked = 0          \
    }
#define DEFINE_SPINLOCK(x) spinlock_t x = SPINLOCK_INITIALIZER
#define DECLARE_SPINLOCK(x) extern spinlock_t x

static inline void spin_lock_init(spinlock_t *lock)
{
    lock->locked = 0;
}

static inline bool spin_lock_try(spinlock_t *lock)
{
    return __sync_lock_test_and_set(&lock->locked, 1) == 0;
}

static inline void spin_lock(spinlock_t *lock)
{
    while (__sync_lock_test_and_set(&lock->locked, 1))
    {
        while (lock->locked)
        {
            cpu_relax();
        }
    }
}

static inline void spin_unlock(spinlock_t *lock)
{
    assert(lock->locked == 1);
    __sync_lock_release(&lock->locked);
}