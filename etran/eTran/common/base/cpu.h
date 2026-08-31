#pragma once

#include <sys/time.h>

#include <stdint.h>
#include <stdlib.h>

#define CACHE_LINE_SIZE 64

static inline void cpu_relax(void)
{
#if __GNUC_PREREQ(10, 0)
#if __has_builtin(__builtin_ia32_pause)
	__builtin_ia32_pause();
#endif
#else
	asm volatile("pause");
#endif
}

/* time related */
static inline uint64_t rdtsc(void)
{
	uint32_t lo, hi;
	__asm__ __volatile__("rdtsc" : "=a"(lo), "=d"(hi));
	return (((uint64_t)hi << 32) | lo);
}

#define get_cycles rdtsc

static inline double get_cycles_per_sec(void)
{
	static double cps = 0;
	if (cps != 0)
	{
		return cps;
	}

	// Take parallel time readings using both rdtsc and gettimeofday.
	// After 10ms have elapsed, take the ratio between these readings.

	struct timeval start_time, stop_time;
	uint64_t start_cycles, stop_cycles, micros;
	double old_cps;

	// There is one tricky aspect, which is that we could get interrupted
	// between calling gettimeofday and reading the cycle counter, in which
	// case we won't have corresponding readings.  To handle this (unlikely)
	// case, compute the overall result repeatedly, and wait until we get
	// two successive calculations that are within 0.1% of each other.
	old_cps = 0;
	while (1)
	{
		if (gettimeofday(&start_time, NULL) != 0)
		{
			exit(1);
		}
		start_cycles = rdtsc();
		while (1)
		{
			if (gettimeofday(&stop_time, NULL) != 0)
			{
				exit(1);
			}
			stop_cycles = rdtsc();
			micros = (stop_time.tv_usec - start_time.tv_usec) +
					 (stop_time.tv_sec - start_time.tv_sec) * 1000000;
			if (micros > 10000)
			{
				cps = (double)(stop_cycles - start_cycles);
				cps = 1000000.0 * cps / (double)(micros);
				break;
			}
		}
		double delta = cps / 1000.0;
		if ((old_cps > (cps - delta)) && (old_cps < (cps + delta)))
		{
			return cps;
		}
		old_cps = cps;
	}
}

static inline double cycles_to_us(uint64_t cycles)
{
	return (double)cycles * 1e6 / get_cycles_per_sec();
}

static inline double cycles_to_ns(uint64_t cycles)
{
	return (double)cycles * 1e9 / get_cycles_per_sec();
}

static inline uint64_t us_to_cycles(double us)
{
	return (uint64_t)(us * get_cycles_per_sec() / 1e6);
}

static inline uint64_t ns_to_cycles(double ns)
{
	return (uint64_t)(ns * get_cycles_per_sec() / 1e9);
}