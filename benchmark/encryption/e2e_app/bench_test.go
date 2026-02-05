package main

// Benchmark encryption/decryption latency for e2e application message sizes
// Reads message sizes from symphony_sizes_boutique.txt and symphony_sizes_hotel.txt
// Run with: go test -bench=. -benchtime=10000x ./benchmark/encryption/e2e_app

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	// Message sizes loaded from files
	boutiqueSizes []int
	hotelSizes    []int

	// Encryption keys
	publicKey  []byte
	privateKey []byte

	// Cached GCM objects
	publicGCM  cipher.AEAD
	privateGCM cipher.AEAD

	// Counter for nonce generation (avoids syscall overhead in benchmarks)
	nonceCounter uint64
)

// generateNonceFromCounter fills the nonce buffer with counter-based values.
// AES-GCM only requires nonces to be unique per key, not random.
func generateNonceFromCounter(nonce []byte) {
	counter := atomic.AddUint64(&nonceCounter, 1)
	for i := range nonce {
		nonce[i] = 0
	}
	binary.LittleEndian.PutUint64(nonce, counter)
}

func init() {
	// Initialize keys
	publicKey, _ = hex.DecodeString("27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796")
	privateKey, _ = hex.DecodeString("9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9")

	// Initialize GCM objects
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

	// Load message sizes from files
	boutiqueSizes, err = loadMessageSizes("symphony_sizes_boutique.txt")
	if err != nil {
		panic(fmt.Sprintf("Failed to load boutique sizes: %v", err))
	}

	hotelSizes, err = loadMessageSizes("symphony_sizes_hotel.txt")
	if err != nil {
		panic(fmt.Sprintf("Failed to load hotel sizes: %v", err))
	}

	fmt.Printf("Loaded %d boutique message sizes, %d hotel message sizes\n", len(boutiqueSizes), len(hotelSizes))
}

func loadMessageSizes(filename string) ([]int, error) {
	// Try multiple paths
	possiblePaths := []string{
		filename,
		filepath.Join("benchmark", "encryption", "e2e_app", filename),
		filepath.Join("e2e_app", filename),
		filepath.Join("..", "e2e_app", filename),
	}

	var file *os.File
	var err error
	for _, path := range possiblePaths {
		file, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if file == nil {
		return nil, fmt.Errorf("failed to find %s in paths: %v", filename, possiblePaths)
	}
	defer file.Close()

	var sizes []int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		size, err := strconv.Atoi(line)
		if err != nil {
			continue // Skip malformed lines
		}
		if size > 0 {
			sizes = append(sizes, size)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return sizes, nil
}

func generateRandomBytes(size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("Failed to generate random bytes: %v", err))
	}
	return data
}

// Sync pools for reducing allocations
var (
	singleNoncePool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 12)
			return &b
		},
	}
	doubleNoncePool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 24)
			return &b
		},
	}
)

// encryptWholeOptimized encrypts data with optimizations
func encryptWholeOptimized(plaintext []byte, isPublic bool) ([]byte, error) {
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	nonceSize := gcm.NonceSize()
	tagSize := gcm.Overhead()

	nonceBuf := singleNoncePool.Get().(*[]byte)
	nonce := *nonceBuf
	generateNonceFromCounter(nonce)

	totalSize := nonceSize + len(plaintext) + tagSize
	result := make([]byte, nonceSize, totalSize)
	copy(result, nonce)

	singleNoncePool.Put(nonceBuf)

	result = gcm.Seal(result, result[:nonceSize], plaintext, nil)

	return result, nil
}

// decryptWholeOptimized decrypts data with optimizations
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

	nonce := encrypted[:nonceSize]
	ciphertextWithTag := encrypted[nonceSize:]

	plaintextSize := len(ciphertextWithTag) - tagSize
	result := make([]byte, 0, plaintextSize)

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

// encryptSplitOptimized encrypts both public and private parts
func encryptSplitOptimized(publicPart, privatePart []byte) ([]byte, []byte, error) {
	nonceSize := publicGCM.NonceSize()
	tagSize := publicGCM.Overhead()

	noncesBuf := doubleNoncePool.Get().(*[]byte)
	nonces := *noncesBuf
	generateNonceFromCounter(nonces[:nonceSize])
	generateNonceFromCounter(nonces[nonceSize:])

	pubCipherSize := nonceSize + len(publicPart) + tagSize
	privCipherSize := nonceSize + len(privatePart) + tagSize

	encryptedPublic := make([]byte, nonceSize, pubCipherSize)
	encryptedPrivate := make([]byte, nonceSize, privCipherSize)

	copy(encryptedPublic, nonces[:nonceSize])
	copy(encryptedPrivate, nonces[nonceSize:])

	doubleNoncePool.Put(noncesBuf)

	encryptedPublic = publicGCM.Seal(encryptedPublic, encryptedPublic[:nonceSize], publicPart, nil)
	encryptedPrivate = privateGCM.Seal(encryptedPrivate, encryptedPrivate[:nonceSize], privatePart, nil)

	return encryptedPublic, encryptedPrivate, nil
}

// decryptSplitOptimized decrypts both public and private parts
func decryptSplitOptimized(encryptedPublic, encryptedPrivate []byte) ([]byte, []byte, error) {
	nonceSize := publicGCM.NonceSize()

	if len(encryptedPublic) < nonceSize {
		return nil, nil, fmt.Errorf("encrypted public data too short")
	}
	if len(encryptedPrivate) < nonceSize {
		return nil, nil, fmt.Errorf("encrypted private data too short")
	}

	tagSize := publicGCM.Overhead()
	pubPlainSize := len(encryptedPublic) - nonceSize - tagSize
	privPlainSize := len(encryptedPrivate) - nonceSize - tagSize

	if pubPlainSize < 0 || privPlainSize < 0 {
		return nil, nil, fmt.Errorf("encrypted data too short for decryption")
	}

	publicPlain := make([]byte, 0, pubPlainSize)
	privatePlain := make([]byte, 0, privPlainSize)

	var err error
	publicPlain, err = publicGCM.Open(publicPlain, encryptedPublic[:nonceSize], encryptedPublic[nonceSize:], nil)
	if err != nil {
		return nil, nil, fmt.Errorf("public decryption failed: %w", err)
	}

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

// TimingEntry represents a single timing measurement with associated message size
type TimingEntry struct {
	LatencyNs   int64
	MessageSize int
}

// writeTimingsWithSize writes timing data with message sizes to a CSV file
func writeTimingsWithSize(filename string, entries []TimingEntry) error {
	dir := "profile_data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "latency_ns,message_size\n")
	for _, e := range entries {
		fmt.Fprintf(f, "%d,%d\n", e.LatencyNs, e.MessageSize)
	}
	return nil
}

// ============================================================================
// BOUTIQUE BENCHMARKS
// ============================================================================

// BenchmarkBoutique_TripleEncryption_Whole measures optimized encryption for whole strings
// with 3x encrypt/decrypt operations per iteration using boutique message sizes
func BenchmarkBoutique_TripleEncryption_Whole(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
	traceSize := len(boutiqueSizes)

	if traceSize == 0 {
		b.Skip("No boutique message sizes loaded")
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		size := boutiqueSizes[idx]

		if size <= 0 {
			continue
		}

		b.StopTimer()
		data1 := generateRandomBytes(size)
		data2 := generateRandomBytes(size)
		data3 := generateRandomBytes(size)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Round 1
		encrypted1, encryptTime1, err := encryptWholeWithTiming(data1, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime1

		_, decryptTime1, err := decryptWholeWithTiming(encrypted1, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime1

		// Round 2
		encrypted2, encryptTime2, err := encryptWholeWithTiming(data2, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime2

		_, decryptTime2, err := decryptWholeWithTiming(encrypted2, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime2

		// Round 3
		encrypted3, encryptTime3, err := encryptWholeWithTiming(data3, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime3

		_, decryptTime3, err := decryptWholeWithTiming(encrypted3, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime3

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("boutique_triple_encryption_whole_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("boutique_triple_encryption_whole_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkBoutique_TripleEncryption_FixedSplit measures optimized encryption for fixed split
// First 8 bytes in first section, rest in second segment
// Pattern: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split 2 times
func BenchmarkBoutique_TripleEncryption_FixedSplit(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
	traceSize := len(boutiqueSizes)

	if traceSize == 0 {
		b.Skip("No boutique message sizes loaded")
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		size := boutiqueSizes[idx]

		// Need at least 9 bytes: 8 for first section, 1 for second
		if size <= 8 {
			continue
		}

		b.StopTimer()
		const splitPoint = 8

		data1 := generateRandomBytes(size)
		publicPart1 := data1[:splitPoint]
		privatePart1 := data1[splitPoint:]

		firstSplitData2 := generateRandomBytes(splitPoint)
		firstSplitData3 := generateRandomBytes(splitPoint)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart1, privatePart1)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the first split
		encrypted2, encTime2, err := encryptWholeWithTiming(firstSplitData2, true)
		if err != nil {
			b.Fatalf("Encryption of first split failed: %v", err)
		}
		totalEncryptTime += encTime2

		_, decTime2, err := decryptWholeWithTiming(encrypted2, true)
		if err != nil {
			b.Fatalf("Decryption of first split failed: %v", err)
		}
		totalDecryptTime += decTime2

		// Step 3: Encrypt/decrypt only the first split
		encrypted3, encTime3, err := encryptWholeWithTiming(firstSplitData3, true)
		if err != nil {
			b.Fatalf("Encryption of first split failed: %v", err)
		}
		totalEncryptTime += encTime3

		_, decTime3, err := decryptWholeWithTiming(encrypted3, true)
		if err != nil {
			b.Fatalf("Decryption of first split failed: %v", err)
		}
		totalDecryptTime += decTime3

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("boutique_triple_encryption_fixed_split_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("boutique_triple_encryption_fixed_split_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// ============================================================================
// HOTEL BENCHMARKS
// ============================================================================

// BenchmarkHotel_TripleEncryption_Whole measures optimized encryption for whole strings
// with 3x encrypt/decrypt operations per iteration using hotel message sizes
func BenchmarkHotel_TripleEncryption_Whole(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
	traceSize := len(hotelSizes)

	if traceSize == 0 {
		b.Skip("No hotel message sizes loaded")
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		size := hotelSizes[idx]

		if size <= 0 {
			continue
		}

		b.StopTimer()
		data1 := generateRandomBytes(size)
		data2 := generateRandomBytes(size)
		data3 := generateRandomBytes(size)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Round 1
		encrypted1, encryptTime1, err := encryptWholeWithTiming(data1, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime1

		_, decryptTime1, err := decryptWholeWithTiming(encrypted1, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime1

		// Round 2
		encrypted2, encryptTime2, err := encryptWholeWithTiming(data2, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime2

		_, decryptTime2, err := decryptWholeWithTiming(encrypted2, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime2

		// Round 3
		encrypted3, encryptTime3, err := encryptWholeWithTiming(data3, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime3

		_, decryptTime3, err := decryptWholeWithTiming(encrypted3, true)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime3

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("hotel_triple_encryption_whole_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("hotel_triple_encryption_whole_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkHotel_TripleEncryption_FixedSplit measures optimized encryption for fixed split
// First 8 bytes in first section, rest in second segment
// Pattern: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split 2 times
func BenchmarkHotel_TripleEncryption_FixedSplit(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
	traceSize := len(hotelSizes)

	if traceSize == 0 {
		b.Skip("No hotel message sizes loaded")
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		size := hotelSizes[idx]

		// Need at least 9 bytes: 8 for first section, 1 for second
		if size <= 8 {
			continue
		}

		b.StopTimer()
		const splitPoint = 8

		data1 := generateRandomBytes(size)
		publicPart1 := data1[:splitPoint]
		privatePart1 := data1[splitPoint:]

		firstSplitData2 := generateRandomBytes(splitPoint)
		firstSplitData3 := generateRandomBytes(splitPoint)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart1, privatePart1)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the first split
		encrypted2, encTime2, err := encryptWholeWithTiming(firstSplitData2, true)
		if err != nil {
			b.Fatalf("Encryption of first split failed: %v", err)
		}
		totalEncryptTime += encTime2

		_, decTime2, err := decryptWholeWithTiming(encrypted2, true)
		if err != nil {
			b.Fatalf("Decryption of first split failed: %v", err)
		}
		totalDecryptTime += decTime2

		// Step 3: Encrypt/decrypt only the first split
		encrypted3, encTime3, err := encryptWholeWithTiming(firstSplitData3, true)
		if err != nil {
			b.Fatalf("Encryption of first split failed: %v", err)
		}
		totalEncryptTime += encTime3

		_, decTime3, err := decryptWholeWithTiming(encrypted3, true)
		if err != nil {
			b.Fatalf("Decryption of first split failed: %v", err)
		}
		totalDecryptTime += decTime3

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("hotel_triple_encryption_fixed_split_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("hotel_triple_encryption_fixed_split_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}
