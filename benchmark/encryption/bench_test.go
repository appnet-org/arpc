package main

// Note: This file uses package main because it needs to be runnable as a standalone test
// The benchmarks will be executed with: go test -bench=. ./benchmark/encryption

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/appnet-org/arpc/pkg/transport"
)

// TraceEntry represents a single entry from the trace file
type TraceEntry struct {
	Op        string
	Key       string
	KeySize   int
	ValueSize int
	TotalSize int // key_size + value_size
}

var (
	traceEntries []TraceEntry
	publicKey    []byte
	privateKey   []byte
	// Cached GCM objects (matching transport package)
	publicGCM  cipher.AEAD
	privateGCM cipher.AEAD
)

func init() {
	// Initialize keys (same as in encryption.go)
	publicKey, _ = hex.DecodeString("27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796")
	privateKey, _ = hex.DecodeString("9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9")

	// Initialize GCM objects using transport package
	if err := transport.InitGCMObjects(publicKey, privateKey); err != nil {
		panic(fmt.Sprintf("Failed to initialize GCM objects: %v", err))
	}

	// Initialize local GCM objects for benchmark (matching transport package)
	var err error
	publicBlock, err := aes.NewCipher(publicKey)
	if err != nil {
		panic(fmt.Sprintf("Failed to create public cipher: %v", err))
	}
	publicGCM, err = cipher.NewGCM(publicBlock)
	if err != nil {
		panic(fmt.Sprintf("Failed to create public GCM: %v", err))
	}

	privateBlock, err := aes.NewCipher(privateKey)
	if err != nil {
		panic(fmt.Sprintf("Failed to create private cipher: %v", err))
	}
	privateGCM, err = cipher.NewGCM(privateBlock)
	if err != nil {
		panic(fmt.Sprintf("Failed to create private GCM: %v", err))
	}

	// Load trace file - try paths relative to repo root or test file location
	var traceFile string
	possiblePaths := []string{
		filepath.Join("benchmark", "meta-kv-trace", "trace.req"), // from repo root
		filepath.Join("..", "..", "meta-kv-trace", "trace.req"),  // from encryption dir
		filepath.Join("..", "meta-kv-trace", "trace.req"),        // alternative
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			traceFile = path
			break
		}
	}

	if traceFile == "" {
		panic("Failed to find trace.req file. Tried: " + fmt.Sprintf("%v", possiblePaths))
	}

	entries, err := loadTrace(traceFile)
	if err != nil {
		panic(fmt.Sprintf("Failed to load trace file %s: %v", traceFile, err))
	}
	traceEntries = entries
}

func loadTrace(filename string) ([]TraceEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []TraceEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry, err := parseTraceLine(line)
		if err != nil {
			continue // Skip malformed lines
		}
		// Skip entries with zero key or value size
		if entry.KeySize <= 0 || entry.ValueSize <= 0 {
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func parseTraceLine(line string) (TraceEntry, error) {
	// Parse URL format: /?op={GET|SET}&key={key}&key_size={size}&value_size={size}
	parsed, err := url.Parse(line)
	if err != nil {
		return TraceEntry{}, err
	}

	params := parsed.Query()
	op := params.Get("op")
	key := params.Get("key")
	keySizeStr := params.Get("key_size")
	valueSizeStr := params.Get("value_size")

	keySize, err := strconv.Atoi(keySizeStr)
	if err != nil {
		return TraceEntry{}, err
	}

	valueSize, err := strconv.Atoi(valueSizeStr)
	if err != nil {
		return TraceEntry{}, err
	}

	return TraceEntry{
		Op:        op,
		Key:       key,
		KeySize:   keySize,
		ValueSize: valueSize,
		TotalSize: keySize + valueSize,
	}, nil
}

func generateRandomBytes(size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("Failed to generate random bytes: %v", err))
	}
	return data
}

// ============================================================================
// OPTIMIZED SPLIT ENCRYPTION FUNCTIONS
// ============================================================================

// Sync pools for reducing allocations
var (
	// Pool for single nonce (12 bytes)
	singleNoncePool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 12)
			return &b
		},
	}
	// Pool for double nonces (24 bytes = 2 * 12)
	doubleNoncePool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 24)
			return &b
		},
	}
)

// encryptWholeOptimized encrypts data with the same optimizations as split:
// 1. Pooled nonce buffer
// 2. Pre-allocated output buffer
func encryptWholeOptimized(plaintext []byte, isPublic bool) ([]byte, error) {
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	nonceSize := gcm.NonceSize()
	tagSize := gcm.Overhead()

	// Get pooled nonce buffer
	nonceBuf := singleNoncePool.Get().(*[]byte)
	nonce := *nonceBuf
	if _, err := rand.Read(nonce); err != nil {
		singleNoncePool.Put(nonceBuf)
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Pre-allocate output buffer with exact capacity
	totalSize := nonceSize + len(plaintext) + tagSize
	result := make([]byte, nonceSize, totalSize)
	copy(result, nonce)

	// Return pooled buffer
	singleNoncePool.Put(nonceBuf)

	// Encrypt, appending to pre-allocated buffer
	result = gcm.Seal(result, result[:nonceSize], plaintext, nil)

	return result, nil
}

// decryptWholeOptimized decrypts data with the same optimizations as split:
// 1. Pre-allocated output buffer
func decryptWholeOptimized(encrypted []byte, isPublic bool) ([]byte, error) {
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	nonceSize := gcm.NonceSize()
	tagSize := gcm.Overhead()

	if len(encrypted) < nonceSize+tagSize {
		return nil, fmt.Errorf("encrypted data too short")
	}

	// Extract nonce and ciphertext
	nonce := encrypted[:nonceSize]
	ciphertextWithTag := encrypted[nonceSize:]

	// Pre-allocate output buffer with exact capacity
	plaintextSize := len(ciphertextWithTag) - tagSize
	result := make([]byte, 0, plaintextSize)

	// Decrypt
	plaintext, err := gcm.Open(result, nonce, ciphertextWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// encryptWholeWithTiming encrypts data and returns timing
func encryptWholeWithTiming(data []byte, isPublic bool) ([]byte, int64, error) {
	start := time.Now()
	encrypted, err := encryptWholeOptimized(data, isPublic)
	elapsed := time.Since(start)
	if err != nil {
		return nil, 0, err
	}
	return encrypted, elapsed.Nanoseconds(), nil
}

// decryptWholeWithTiming decrypts data and returns timing
func decryptWholeWithTiming(encrypted []byte, isPublic bool) ([]byte, int64, error) {
	start := time.Now()
	decrypted, err := decryptWholeOptimized(encrypted, isPublic)
	elapsed := time.Since(start)
	if err != nil {
		return nil, 0, err
	}
	return decrypted, elapsed.Nanoseconds(), nil
}

// encryptSplitOptimized encrypts both public and private parts with optimizations:
// 1. Batch nonce generation (single rand.Read for both nonces)
// 2. Pre-allocated output buffers
// 3. Reduced allocations
func encryptSplitOptimized(publicPart, privatePart []byte) ([]byte, []byte, error) {
	nonceSize := publicGCM.NonceSize() // 12 bytes
	tagSize := publicGCM.Overhead()    // 16 bytes

	// Get pooled nonce buffer and generate both nonces in one syscall
	noncesBuf := doubleNoncePool.Get().(*[]byte)
	nonces := *noncesBuf
	if _, err := rand.Read(nonces); err != nil {
		doubleNoncePool.Put(noncesBuf)
		return nil, nil, fmt.Errorf("failed to generate nonces: %w", err)
	}

	// Pre-calculate exact sizes
	pubCipherSize := nonceSize + len(publicPart) + tagSize
	privCipherSize := nonceSize + len(privatePart) + tagSize

	// Pre-allocate output buffers with exact capacity
	encryptedPublic := make([]byte, nonceSize, pubCipherSize)
	encryptedPrivate := make([]byte, nonceSize, privCipherSize)

	// Copy nonces to output buffers
	copy(encryptedPublic, nonces[:nonceSize])
	copy(encryptedPrivate, nonces[nonceSize:])

	// Return pooled buffer
	doubleNoncePool.Put(noncesBuf)

	// Encrypt in-place, appending to the nonce
	encryptedPublic = publicGCM.Seal(encryptedPublic, encryptedPublic[:nonceSize], publicPart, nil)
	encryptedPrivate = privateGCM.Seal(encryptedPrivate, encryptedPrivate[:nonceSize], privatePart, nil)

	return encryptedPublic, encryptedPrivate, nil
}

// decryptSplitOptimized decrypts both public and private parts with optimizations:
// 1. Pre-allocated output buffers
// 2. Reduced allocations
func decryptSplitOptimized(encryptedPublic, encryptedPrivate []byte) ([]byte, []byte, error) {
	nonceSize := publicGCM.NonceSize()

	// Validate minimum sizes
	if len(encryptedPublic) < nonceSize {
		return nil, nil, fmt.Errorf("encrypted public data too short")
	}
	if len(encryptedPrivate) < nonceSize {
		return nil, nil, fmt.Errorf("encrypted private data too short")
	}

	// Pre-allocate output buffers (plaintext size = ciphertext - nonce - tag)
	tagSize := publicGCM.Overhead()
	pubPlainSize := len(encryptedPublic) - nonceSize - tagSize
	privPlainSize := len(encryptedPrivate) - nonceSize - tagSize

	if pubPlainSize < 0 || privPlainSize < 0 {
		return nil, nil, fmt.Errorf("encrypted data too short for decryption")
	}

	// Pre-allocate with exact capacity
	publicPlain := make([]byte, 0, pubPlainSize)
	privatePlain := make([]byte, 0, privPlainSize)

	// Decrypt public
	var err error
	publicPlain, err = publicGCM.Open(publicPlain, encryptedPublic[:nonceSize], encryptedPublic[nonceSize:], nil)
	if err != nil {
		return nil, nil, fmt.Errorf("public decryption failed: %w", err)
	}

	// Decrypt private
	privatePlain, err = privateGCM.Open(privatePlain, encryptedPrivate[:nonceSize], encryptedPrivate[nonceSize:], nil)
	if err != nil {
		return nil, nil, fmt.Errorf("private decryption failed: %w", err)
	}

	return publicPlain, privatePlain, nil
}

// encryptSplitWithTiming encrypts both parts and returns timing
func encryptSplitWithTiming(publicPart, privatePart []byte) ([]byte, []byte, int64, error) {
	start := time.Now()
	encPub, encPriv, err := encryptSplitOptimized(publicPart, privatePart)
	elapsed := time.Since(start)
	if err != nil {
		return nil, nil, 0, err
	}
	return encPub, encPriv, elapsed.Nanoseconds(), nil
}

// decryptSplitWithTiming decrypts both parts and returns timing
func decryptSplitWithTiming(encryptedPublic, encryptedPrivate []byte) ([]byte, []byte, int64, error) {
	start := time.Now()
	pubPlain, privPlain, err := decryptSplitOptimized(encryptedPublic, encryptedPrivate)
	elapsed := time.Since(start)
	if err != nil {
		return nil, nil, 0, err
	}
	return pubPlain, privPlain, elapsed.Nanoseconds(), nil
}

// writeTimings writes timing data (in nanoseconds) to a file, one value per line
func writeTimings(filename string, timings []int64) error {
	// Create subdirectory for profile data
	dir := "profile_data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to file in subdirectory
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, t := range timings {
		fmt.Fprintf(f, "%d\n", t)
	}
	return nil
}

// BenchmarkEncryption_Whole measures optimized encryption for whole strings
// Uses the same optimizations as split: pooled nonce, pre-allocated buffers
func BenchmarkEncryption_Whole(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 0 {
			continue
		}

		// Generate test data for this trace entry (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)
		b.StartTimer()

		// Encrypt with optimized function
		encrypted, encryptTime, err := encryptWholeWithTiming(data, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt with optimized function
		_, decryptTime, err := decryptWholeWithTiming(encrypted, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_whole_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_whole_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkEncryption_RandomSplit measures optimized encryption for randomly split strings
func BenchmarkEncryption_RandomSplit(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 1 {
			continue // Need at least 2 bytes to split
		}

		// Generate test data and calculate random split point (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)
		// Generate random split point between 1 and size-1
		splitPoint := 1
		if size > 2 {
			splitPointBytes := make([]byte, 4)
			rand.Read(splitPointBytes)
			splitPoint = 1 + (int(splitPointBytes[0])|int(splitPointBytes[1])<<8|int(splitPointBytes[2])<<16|int(splitPointBytes[3])<<24)%(size-1)
			if splitPoint < 1 {
				splitPoint = 1
			}
			if splitPoint >= size {
				splitPoint = size - 1
			}
		}
		publicPart := data[:splitPoint]
		privatePart := data[splitPoint:]
		b.StartTimer()

		// Encrypt both parts with optimized function
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt both parts with optimized function
		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_random_split_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_random_split_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkEncryption_EqualSplit measures optimized encryption for equal-length substrings
func BenchmarkEncryption_EqualSplit(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 1 {
			continue
		}

		// Generate test data and split into two equal parts (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)
		splitPoint := size / 2

		publicPart := data[:splitPoint]
		privatePart := data[splitPoint:]
		b.StartTimer()

		// Encrypt both parts with optimized function
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt both parts with optimized function
		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_equal_split_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_equal_split_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkEncryption_RandomAlignedSplit measures optimized encryption for randomly split strings
// with the split point aligned to 16 bytes (AES block size)
func BenchmarkEncryption_RandomAlignedSplit(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 16 {
			continue // Need at least 16 bytes to have an aligned split
		}

		// Generate test data and calculate aligned random split point (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)

		// Calculate number of possible 16-byte aligned split points
		// Split points can be at 16, 32, 48, ... up to the largest multiple < size
		numAlignedPoints := (size - 1) / 16 // number of valid 16-byte aligned split points
		if numAlignedPoints < 1 {
			b.StartTimer()
			continue
		}

		// Generate random aligned split point
		splitPointBytes := make([]byte, 4)
		rand.Read(splitPointBytes)
		alignedIndex := 1 + (int(splitPointBytes[0])|int(splitPointBytes[1])<<8)%numAlignedPoints
		splitPoint := alignedIndex * 16

		if splitPoint >= size {
			splitPoint = (numAlignedPoints) * 16
		}
		if splitPoint < 16 {
			splitPoint = 16
		}

		publicPart := data[:splitPoint]
		privatePart := data[splitPoint:]
		b.StartTimer()

		// Encrypt both parts with optimized function
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt both parts with optimized function
		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_random_aligned_split_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_random_aligned_split_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkEncryption_2_8_Split measures optimized encryption for 20% public / 80% private split
func BenchmarkEncryption_2_8_Split(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 1 {
			continue
		}

		// Generate test data and split 20% public / 80% private (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)
		splitPoint := size * 2 / 10 // 20% for public part
		if splitPoint < 1 {
			splitPoint = 1
		}
		if splitPoint >= size {
			splitPoint = size - 1
		}
		publicPart := data[:splitPoint]
		privatePart := data[splitPoint:]
		b.StartTimer()

		// Encrypt both parts with optimized function
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt both parts with optimized function
		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_2_8_split_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_2_8_split_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkEncryption_KeyValueSplit measures optimized encryption for key/value split
// where the key is in one segment (public) and the value is in another segment (private)
func BenchmarkEncryption_KeyValueSplit(b *testing.B) {
	encryptTimings := make([]int64, 0, b.N)
	decryptTimings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]

		keySize := entry.KeySize
		valueSize := entry.ValueSize

		if keySize <= 0 || valueSize <= 0 {
			continue // Need both key and value to have content
		}

		// Generate test data for key and value separately (excluded from timing)
		b.StopTimer()
		keyData := generateRandomBytes(keySize)
		valueData := generateRandomBytes(valueSize)
		b.StartTimer()

		// Encrypt both parts with optimized function
		// Key as public part, Value as private part
		encryptedKey, encryptedValue, encryptTime, err := encryptSplitWithTiming(keyData, valueData)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt both parts with optimized function
		_, _, decryptTime, err := decryptSplitWithTiming(encryptedKey, encryptedValue)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		decryptTimings = append(decryptTimings, decryptTime)
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimings("encryption_key_value_split_encrypt_times.txt", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimings("encryption_key_value_split_decrypt_times.txt", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}
