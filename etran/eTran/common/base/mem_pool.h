#pragma once

#include <assert.h>

#include <base/cpu.h>
#include <base/math.h>

#include <iostream>
#include <mutex>
#include <vector>
#include <memory>

#define GBytes(x) ((size_t)x * 1024 * 1024 * 1024)
#define MBytes(x) ((size_t)x * 1024 * 1024)
#define KBytes(x) ((size_t)x * 1024)

static constexpr size_t HugePageSize = MBytes(2);

struct shm_region_t {
    uint32_t key;
    uint8_t *addr;
    size_t size;

    shm_region_t(uint32_t key, uint8_t *addr, size_t size) : key(key), addr(addr), size(size) {}
};

struct Buffer {
    public:
        uint8_t *_buf;
        size_t actual_size;
        size_t class_size;
    
    Buffer(uint8_t *buf, size_t actual_size, size_t class_size) : _buf(buf), actual_size(actual_size), class_size(class_size) {
    }

    Buffer() {}

    ~Buffer() {}

    std::string to_string() {
        return "Buffer: " + std::to_string((uint64_t)_buf) + " class_size: " + std::to_string(class_size);
    }
};

/* Huge-page backed up memory pool */
class Mempool {
public:
    static constexpr size_t nr_class_type = 18;
    static constexpr size_t min_class_size = 64;
    static constexpr size_t class_type_shift = 6;
    static constexpr size_t max_class_size = MBytes(8);

    size_t prev_alloc_class_size;

    std::vector<shm_region_t> shm_regions;
    std::vector<Buffer> free_list[nr_class_type];

    Buffer alloc_buffer(size_t size);

    inline size_t get_class_type(size_t size) {
        assert(size >= 1 && size <= max_class_size);
        return msb_index( static_cast<int>((size-1) >> class_type_shift));
    }

    inline size_t class_type_to_size(size_t class_type) {
        return min_class_size * (1 << class_type);
    }

    inline void split(size_t class_type) {
        assert(!free_list[class_type].empty());
        assert(free_list[class_type - 1].empty());

        Buffer buffer = free_list[class_type].back();
        free_list[class_type].pop_back();

        Buffer buffer_0 = Buffer(buffer._buf, 0, buffer.class_size / 2);
        Buffer buffer_1 = Buffer(buffer._buf + buffer.class_size / 2, 0, buffer.class_size / 2);

        free_list[class_type - 1].push_back(buffer_0);
        free_list[class_type - 1].push_back(buffer_1);
    }

    inline Buffer alloc_from_class(size_t class_type, size_t actual_size) {
        if (free_list[class_type].empty()) {
            return Buffer(nullptr, 0, 0);
        }

        Buffer buf = free_list[class_type].back();
        buf.actual_size = actual_size;
        free_list[class_type].pop_back();
        stats.nr_freed[class_type]--;
        return buf;
    }

    inline void free_buffer(Buffer &buf) {
        size_t class_type = get_class_type(buf.class_size);
        buf.actual_size = 0;
        free_list[class_type].push_back(buf);
        stats.nr_freed[class_type]++;
    }

    inline bool valid_buffer(Buffer &buf) {
        return buf._buf != nullptr && buf.actual_size != 0 && buf.actual_size <= buf.class_size;
    }

    void dump_stats(void);

    Buffer alloc_raw(size_t size);

    bool alloc_slow(size_t alloc_size);

    struct {
        size_t nr_freed[nr_class_type];
        size_t shm_reg;
    } stats;

    Mempool(size_t initial_size);
    ~Mempool();
};