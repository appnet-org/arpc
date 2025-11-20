package common

import "sync"

const DefaultMaxSize = 65536

// BufferPool provides a pool of byte buffers for reuse
type BufferPool struct {
	pool        *sync.Pool
	defaultSize int
	maxSize     int
}

// NewBufferPool creates a new buffer pool with the specified default size.
// Buffers larger than maxSize (default 64KB) will not be pooled to avoid memory bloat.
func NewBufferPool(defaultSize int) *BufferPool {
	return NewBufferPoolWithMaxSize(defaultSize, DefaultMaxSize)
}

// NewBufferPoolWithMaxSize creates a new buffer pool with the specified default and maximum sizes.
// Buffers larger than maxSize will not be pooled to avoid memory bloat.
func NewBufferPoolWithMaxSize(defaultSize, maxSize int) *BufferPool {
	return &BufferPool{
		pool: &sync.Pool{
			New: func() any {
				buf := make([]byte, defaultSize)
				return &buf
			},
		},
		defaultSize: defaultSize,
		maxSize:     maxSize,
	}
}

// Get retrieves a buffer from the pool.
// The returned buffer has at least the default size, but may be larger.
func (bp *BufferPool) Get() []byte {
	ptr := bp.pool.Get().(*[]byte)
	return *ptr
}

// GetSize retrieves a buffer from the pool with at least the specified size.
// If the pooled buffer is smaller than the requested size, a new buffer is allocated.
func (bp *BufferPool) GetSize(size int) []byte {
	buf := bp.Get()
	if cap(buf) < size {
		// Return the small buffer to pool and allocate a new one
		bp.Put(buf)
		return make([]byte, size)
	}
	// Reslice to the requested size
	return buf[:size]
}

// Put returns a buffer to the pool.
// Only buffers with capacity <= maxSize will be pooled to avoid memory bloat.
func (bp *BufferPool) Put(buf []byte) {
	// Only put back buffers that are not too large to avoid memory bloat
	if cap(buf) <= bp.maxSize {
		// Allocate a new pointer to avoid pointing to stack-allocated parameter
		bufPtr := new([]byte)
		*bufPtr = buf
		bp.pool.Put(bufPtr)
	}
}

// DefaultSize returns the default buffer size for this pool
func (bp *BufferPool) DefaultSize() int {
	return bp.defaultSize
}

// MaxSize returns the maximum buffer size that will be pooled
func (bp *BufferPool) MaxSize() int {
	return bp.maxSize
}
