#pragma once

#include <stdint.h>
#include <stddef.h>
#include <assert.h>

#include <string>

typedef uintptr_t shmptr_t;

struct shm_wrapper {
    std::string name;
    int fd;

    uint32_t size;
    void *addr;
};

struct shm_wrapper *shm_wrapper_create(const std::string &__name, size_t __len);
struct shm_wrapper *shm_wrapper_attach(const std::string &__name, size_t __len);

void shm_wrapper_destroy(struct shm_wrapper *);
void shm_wrapper_detach(struct shm_wrapper *);

// Given a pointer, return the offset from the start of the shared memory region
static inline shmptr_t ptr_to_shmptr(void *ptr, struct shm_wrapper *shm)
{
    shmptr_t offset = (uintptr_t)ptr - (uintptr_t)shm->addr;
    assert(offset < shm->size);
    return offset;
}

// Given an offset from the start of the shared memory region, return the pointer
static inline uintptr_t shmptr_to_ptr(shmptr_t shmptr, struct shm_wrapper *shm)
{
    assert(shmptr < shm->size);
    return (uintptr_t)shm->addr + shmptr;
}