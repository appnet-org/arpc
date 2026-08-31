#pragma once

#include <stdint.h>

/* Atomic types */
typedef struct {
    volatile int32_t counter;
} atomic32_t;

typedef struct {
    volatile int64_t counter;
} atomic64_t;