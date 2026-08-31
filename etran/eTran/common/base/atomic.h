#pragma once

#include <stdint.h>
#include <stdbool.h>

#include "base/compiler.h"
#include "base/types.h"

#define mb() asm volatile("mfence" ::: "memory")

#define rmb() barrier()

#define wmb() barrier()

#define load_acquire(p) ({            \
    typeof(*p) __p = ACCESS_ONCE(*p); \
    barrier();                        \
    __p;                              \
})

#define store_release(p, v)  \
    do                       \
    {                        \
        barrier();           \
        ACCESS_ONCE(*p) = v; \
    } while (0)

#define atomic32_init(v) atomic32_set(v, 0)
#define atomic64_init(v) atomic64_set(v, 0)

/* atomic32 */
static inline int atomic32_read(const atomic32_t *v)
{
    return *((volatile int32_t *)&v->counter);
}

static inline void atomic32_set(atomic32_t *v, int i)
{
    *((volatile int32_t *)&v->counter) = i;
}

static inline void atomic32_add(atomic32_t *v, int i)
{
    __sync_fetch_and_add(&v->counter, i);
}

static inline void atomic32_sub(atomic32_t *v, int i)
{
    __sync_fetch_and_sub(&v->counter, i);
}

static inline void atomic32_inc(atomic32_t *v)
{
    __sync_fetch_and_add(&v->counter, 1);
}

static inline void atomic32_dec(atomic32_t *v)
{
    __sync_fetch_and_sub(&v->counter, 1);
}

static inline int atomic32_add_return(atomic32_t *v, int i)
{
    return __sync_add_and_fetch(&v->counter, i);
}

static inline int atomic32_sub_return(atomic32_t *v, int i)
{
    return __sync_sub_and_fetch(&v->counter, i);
}

static inline int atomic32_inc_return(atomic32_t *v)
{
    return __sync_add_and_fetch(&v->counter, 1);
}

static inline int atomic32_dec_return(atomic32_t *v)
{
    return __sync_sub_and_fetch(&v->counter, 1);
}

static inline bool atomic32_dec_and_test(atomic32_t *v)
{
    return __sync_sub_and_fetch(&v->counter, 1) == 0;
}

static inline bool atomic32_sub_and_test(atomic32_t *v, int i)
{
    return __sync_sub_and_fetch(&v->counter, i) == 0;
}

static inline bool atomic32_cmpxhg(atomic32_t *v, int oldv, int newv)
{
    return __sync_bool_compare_and_swap(&v->counter, oldv, newv);
}

static inline void atomic32_or(atomic32_t *v, int32_t i)
{
    __sync_fetch_and_or(&v->counter, i);
}

static inline void atomic32_andnot(atomic32_t *v, int32_t i)
{
    __sync_fetch_and_and(&v->counter, ~i);
}

/* atomic64 */
static inline int64_t atomic64_read(const atomic64_t *v)
{
    return *((volatile int64_t *)&v->counter);
}

static inline void atomic64_set(atomic64_t *v, int64_t i)
{
    *((volatile int64_t *)&v->counter) = i;
}

static inline void atomic64_add(atomic64_t *v, int64_t i)
{
    __sync_fetch_and_add(&v->counter, i);
}

static inline void atomic64_sub(atomic64_t *v, int64_t i)
{
    __sync_fetch_and_sub(&v->counter, i);
}

static inline void atomic64_inc(atomic64_t *v)
{
    __sync_fetch_and_add(&v->counter, 1);
}

static inline void atomic64_dec(atomic64_t *v)
{
    __sync_fetch_and_sub(&v->counter, 1);
}

static inline int64_t atomic64_return_add(atomic64_t *v, int64_t i)
{
    return __sync_fetch_and_add(&v->counter, i);
}

static inline int64_t atomic64_add_return(atomic64_t *v, int64_t i)
{
    return __sync_add_and_fetch(&v->counter, i);
}

static inline int64_t atomic64_sub_return(atomic64_t *v, int64_t i)
{
    return __sync_sub_and_fetch(&v->counter, i);
}

static inline int64_t atomic64_inc_return(atomic64_t *v)
{
    return __sync_add_and_fetch(&v->counter, 1);
}

static inline int64_t atomic64_dec_return(atomic64_t *v)
{
    return __sync_sub_and_fetch(&v->counter, 1);
}

static inline bool atomic64_dec_and_test(atomic64_t *v)
{
    return __sync_sub_and_fetch(&v->counter, 1) == 0;
}

static inline bool atomic64_sub_and_test(atomic64_t *v, int i)
{
    return __sync_sub_and_fetch(&v->counter, i) == 0;
}

static inline bool atomic64_cmpxhg(atomic64_t *v, int64_t oldv, int64_t newv)
{
    return __sync_bool_compare_and_swap(&v->counter, oldv, newv);
}

static inline void atomic64_or(atomic64_t *v, int64_t i)
{
    __sync_fetch_and_or(&v->counter, i);
}

/* Is it correct ? */
static inline void atomic64_set_release(atomic64_t *v, int64_t i)
{
    __sync_synchronize();
    *((volatile int64_t *)&v->counter) = i;
}

static inline void atomic64_andnot(atomic64_t *v, int i)
{
    __sync_fetch_and_and(&v->counter, ~i);
}