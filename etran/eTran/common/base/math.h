#pragma once

#include <stdint.h>

#include <base/stddef.h>

#define CEIL_DIV(a, b) (((a) + (b) - 1) / (b))

static inline size_t round_up(size_t x, size_t power_of_two_number) {
    assert (is_power_of_two(power_of_two_number));
    return (x + power_of_two_number - 1) & ~(power_of_two_number - 1);
}

/// Return the index of the most significant bit of x. The index of the 2^0
/// bit is 1. (x = 0 returns 0, x = 1 returns 1.)
static inline size_t msb_index(int x)
{
  assert(x < INT32_MAX / 2);
  int index;
  asm("bsrl %1, %0" : "=r"(index) : "r"(x << 1));
  return static_cast<size_t>(index);
}