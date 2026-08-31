/* Copyright (c) 2019-2022 Stanford University
 *
 * Permission to use, copy, modify, and/or distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

#ifndef _TEST_UTILS_H
#define _TEST_UTILS_H

#include <netinet/in.h>

#include "homa.h"

#ifdef __cplusplus
extern "C"
{
#endif

#define sizeof32(type) static_cast<int>(sizeof(type))

extern int     check_buffer(void *buffer, size_t length);
extern int     check_message(struct homa_recvmsg_args *control,
	           char *region, size_t length, int skip);
extern int     get_int(const char *s, const char *msg);
extern void    pin_thread(int core);
extern void    print_dist(uint64_t times[], int count);
extern void    seed_buffer(void *buffer, size_t length, int seed);
extern const char*
               print_address(const sockaddr_in_union *addr);
extern double  to_seconds(uint64_t cycles);

#ifdef __cplusplus
}
#endif

#endif /* _TEST_UTILS_H */
