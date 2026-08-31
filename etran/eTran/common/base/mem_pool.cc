#include "mem_pool.h"

#include <iostream>
#include <sstream>
#include <cstring>

#include <sys/mman.h>
#include <sys/ipc.h>
#include <sys/shm.h>

Buffer Mempool::alloc_buffer(size_t size)
{
    size_t class_type = get_class_type(size);
    int ret;
    if (likely(!free_list[class_type].empty())) {
        return alloc_from_class(class_type, size);
    }
    else {
        // find in larger class first
        size_t search_class_type = class_type + 1;
        while (search_class_type < nr_class_type && free_list[search_class_type].empty())
            search_class_type++;

        if (search_class_type == nr_class_type)
        {
            prev_alloc_class_size <<= 1;
            ret = alloc_slow(prev_alloc_class_size);
            if (ret == true)
                search_class_type = nr_class_type - 1;
            else
            {
                prev_alloc_class_size >>= 1;
                return Buffer(nullptr, 0, 0);
            }
        }

        while (search_class_type != class_type)
        {
            split(search_class_type);
            search_class_type--;
        }
        return alloc_from_class(class_type, size);
    }
}

void Mempool::dump_stats(void)
{
    std::cout << "Mempool stats: " << std::endl;
    for (size_t i = 0; i < nr_class_type; i++)
    {
        std::cout << "class_type: " << i << " nr_freed: " << stats.nr_freed[i] << std::endl;
    }
}

Buffer Mempool::alloc_raw(size_t size)
{
    std::ostringstream xmsg;
    int shm_id, shm_key;
    size = round_up(size, HugePageSize);

    while (true)
    {
        shm_key = std::rand() % (1 << 30);
        shm_key = std::abs(shm_key);
        
        shm_id = shmget(shm_key, size, IPC_CREAT | IPC_EXCL | 0666 | SHM_HUGETLB);

        if (shm_id == -1)
        {
            switch (errno)
            {
            case EEXIST:
                continue;

            case EACCES:
                xmsg << "HugeAlloc: SHM allocation error. "
                     << "Insufficient permissions.";
                throw std::runtime_error(xmsg.str());

            case EINVAL:
                xmsg << "HugeAlloc: SHM allocation error: SHMMAX/SHMIN "
                     << "mismatch. size = " << std::to_string(size) << " ("
                     << std::to_string(size / MBytes(1)) << " MB).";
                throw std::runtime_error(xmsg.str());

            case ENOMEM:
                fprintf(stderr, "HugeAlloc: SHM allocation error: "
                                "Insufficient memory. size = %zu (%zu MB).\n",
                        size, size / MBytes(1));
                return Buffer(nullptr, 0, 0);

            default:
                xmsg << "HugeAlloc: Unexpected SHM malloc error "
                     << strerror(errno);
                throw std::runtime_error(xmsg.str());
            }
        }
        else
        {
            // shm_key worked. Break out of the while loop.
            break;
        }
    }

    uint8_t *shm_buf = static_cast<uint8_t *>(shmat(shm_id, nullptr, 0));
    assert(shm_buf != nullptr);

    // Mark the SHM region for deletion when this process exits
    shmctl(shm_id, IPC_RMID, nullptr);

    // Save the SHM region so we can free it later
    shm_regions.push_back(
        shm_region_t(shm_key, shm_buf, size));
    stats.shm_reg += size;

    // buffer.class_size is invalid because we didn't allocate from a class
    return Buffer(shm_buf, 0, SIZE_MAX);
}

bool Mempool::alloc_slow(size_t alloc_size)
{
    assert(alloc_size >= max_class_size);

    Buffer buffer = alloc_raw(alloc_size);
    if (buffer._buf == nullptr)
    {
        return false;
    }
    size_t nr_buffer = alloc_size / max_class_size;
    for (size_t i = 0; i < nr_buffer; i++)
    {
        uint8_t *_buf = buffer._buf + i * max_class_size;
        free_list[nr_class_type - 1].push_back(Buffer(_buf, 0, max_class_size));
    }
    return true;
}

Mempool::Mempool(size_t initial_size)
{
    prev_alloc_class_size = initial_size < max_class_size ? initial_size : max_class_size;
}

Mempool::~Mempool()
{
    for (auto &shm_region : shm_regions)
    {
        const int ret =
            shmdt(static_cast<void *>(const_cast<uint8_t *>(shm_region.addr)));
        if (ret != 0)
        {
            fprintf(stderr, "HugeAlloc: Error freeing SHM buf for key %d.\n",
                    shm_region.key);
            exit(-1);
        }
    }
}
