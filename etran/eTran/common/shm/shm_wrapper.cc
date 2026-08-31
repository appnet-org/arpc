#include <shm/shm_wrapper.h>

#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>

#include <stdio.h>
#include <fcntl.h>
#include <unistd.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <string>

struct shm_wrapper *_shm_wrapper_create(const std::string &__name, size_t __len, int __oflag, mode_t __mode)
{
    int fd;
    void *addr;
    struct shm_wrapper *mem = NULL;

    mem = (struct shm_wrapper *)calloc(1, sizeof(struct shm_wrapper));
    if (mem == NULL) {
        fprintf(stderr, "Failed to allocate memory for shared memory\n");
        return NULL;
    }

    fd = shm_open(__name.c_str(), __oflag, __mode);

    if (fd < 0) {
        fprintf(stderr, "Failed to open shared memory\n");
        goto err;
    }

    if (ftruncate(fd, __len) < 0) {
        fprintf(stderr, "Failed to truncate shared memory\n");
        goto err;
    }

    addr = mmap(NULL, __len, PROT_READ | PROT_WRITE, MAP_SHARED | MAP_POPULATE, fd, 0);

    if (addr == MAP_FAILED) {
        fprintf(stderr, "Failed to map shared memory\n");
        goto err;
    }

    mem->name = __name;
    mem->fd = fd;
    mem->addr = addr;
    mem->size = __len;

    close(fd);

    return mem;
err:
    close(fd);
    free(mem);
    return NULL;
}

struct shm_wrapper *shm_wrapper_create(const std::string &__name, size_t __len)
{
    struct shm_wrapper *mem;
    shm_unlink(__name.c_str());
    mode_t old_mask = umask(0);

    mem = _shm_wrapper_create(__name, __len, O_RDWR | O_CREAT | O_EXCL, 0666);
    if (!mem) {
        umask(old_mask);
        return NULL;
    }
    umask(old_mask);
    return mem;
}

struct shm_wrapper *shm_wrapper_attach(const std::string &__name, size_t __len)
{
    struct shm_wrapper *mem;

    mem = _shm_wrapper_create(__name, __len, O_RDWR, 0);
    if (!mem) {
        return NULL;
    }

    return mem;
}

void shm_wrapper_destroy(struct shm_wrapper *mem)
{
    if (!mem)
        return;
    if (munmap(mem->addr, mem->size) < 0) {
        fprintf(stderr, "Failed to unmap shared memory\n");
    }
    /* unlink this shm */
    if (shm_unlink(mem->name.c_str()) < 0) {
        fprintf(stderr, "Failed to unlink shared memory %s\n", mem->name.c_str());
        return;
    }
    printf("%s has been destroyed.\n", mem->name.c_str());
    free(mem);

    return;
}

void shm_wrapper_detach(struct shm_wrapper *mem)
{
    if (!mem)
        return;
    /* we just unmap this memory */
    if (munmap(mem->addr, mem->size) < 0) {
        fprintf(stderr, "Failed to unmap shared memory\n");
    }
    printf("%s has been detached.\n", mem->name.c_str());
    free(mem);
    
    return;
}