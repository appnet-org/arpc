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

// encryptSegment encrypts plaintext using AES-GCM.
// Returns [nonce(12 bytes)][ciphertext+tag(16 bytes)].
// This is a copy of the function from encryption.go since it's not exported.
// Note: Nonce is NOT reusable - each encryption must use a unique nonce for security.
// isPublic: true to use public key GCM, false to use private key GCM
func encryptSegment(plaintext []byte, isPublic bool) ([]byte, error) {
	// Get cached GCM object (reuses cipher and GCM objects)
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	// Generate random nonce (12 bytes for standard GCM)
	// IMPORTANT: Nonce must be unique for each encryption - never reuse!
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	// Seal appends the ciphertext and tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// decryptSegment decrypts encrypted data using AES-GCM.
// Expects input format: [nonce(12 bytes)][ciphertext+tag(16 bytes)].
// Returns plaintext or error if authentication fails.
// This is a copy of the function from encryption.go since it's not exported.
// isPublic: true to use public key GCM, false to use private key GCM
func decryptSegment(encrypted []byte, isPublic bool) ([]byte, error) {
	// Get cached GCM object (reuses cipher and GCM objects)
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	// Validate minimum size (nonce + tag)
	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, fmt.Errorf("encrypted data too short: %d bytes (expected at least %d)", len(encrypted), nonceSize)
	}

	// Extract nonce and ciphertext+tag
	nonce := encrypted[:nonceSize]
	ciphertextWithTag := encrypted[nonceSize:]

	// Decrypt and authenticate
	plaintext, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// encryptWithTiming encrypts data and returns the encrypted data and timing in nanoseconds
func encryptWithTiming(data []byte, isPublic bool) ([]byte, int64, error) {
	start := time.Now()
	encrypted, err := encryptSegment(data, isPublic)
	elapsed := time.Since(start)
	if err != nil {
		return nil, 0, err
	}
	return encrypted, elapsed.Nanoseconds(), nil
}

// decryptWithTiming decrypts data and returns the decrypted data and timing in nanoseconds
func decryptWithTiming(encrypted []byte, isPublic bool) ([]byte, int64, error) {
	start := time.Now()
	decrypted, err := decryptSegment(encrypted, isPublic)
	elapsed := time.Since(start)
	if err != nil {
		return nil, 0, err
	}
	return decrypted, elapsed.Nanoseconds(), nil
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

// BenchmarkEncryption_Whole measures encryption and decryption time for whole strings
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

		// Encrypt with public key
		encrypted, encryptTime, err := encryptWithTiming(data, true)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		encryptTimings = append(encryptTimings, encryptTime)

		// Decrypt with public key
		_, decryptTime, err := decryptWithTiming(encrypted, true)
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

// BenchmarkEncryption_RandomSplit measures encryption and decryption time for randomly split strings
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

		// Encrypt public part with public key
		encryptedPublic, encryptPublicTime, err := encryptWithTiming(publicPart, true)
		if err != nil {
			b.Fatalf("Public encryption failed: %v", err)
		}

		// Encrypt private part with private key
		encryptedPrivate, encryptPrivateTime, err := encryptWithTiming(privatePart, false)
		if err != nil {
			b.Fatalf("Private encryption failed: %v", err)
		}

		totalEncryptTime := encryptPublicTime + encryptPrivateTime
		encryptTimings = append(encryptTimings, totalEncryptTime)

		// Decrypt public part with public key
		_, decryptPublicTime, err := decryptWithTiming(encryptedPublic, true)
		if err != nil {
			b.Fatalf("Public decryption failed: %v", err)
		}

		// Decrypt private part with private key
		_, decryptPrivateTime, err := decryptWithTiming(encryptedPrivate, false)
		if err != nil {
			b.Fatalf("Private decryption failed: %v", err)
		}

		totalDecryptTime := decryptPublicTime + decryptPrivateTime
		decryptTimings = append(decryptTimings, totalDecryptTime)
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

// BenchmarkEncryption_EqualSplit measures encryption and decryption time for equal-length substrings
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

		// Encrypt public part with public key
		encryptedPublic, encryptPublicTime, err := encryptWithTiming(publicPart, true)
		if err != nil {
			b.Fatalf("Public encryption failed: %v", err)
		}

		// Encrypt private part with private key
		encryptedPrivate, encryptPrivateTime, err := encryptWithTiming(privatePart, false)
		if err != nil {
			b.Fatalf("Private encryption failed: %v", err)
		}

		totalEncryptTime := encryptPublicTime + encryptPrivateTime
		encryptTimings = append(encryptTimings, totalEncryptTime)

		// Decrypt public part with public key
		_, decryptPublicTime, err := decryptWithTiming(encryptedPublic, true)
		if err != nil {
			b.Fatalf("Public decryption failed: %v", err)
		}

		// Decrypt private part with private key
		_, decryptPrivateTime, err := decryptWithTiming(encryptedPrivate, false)
		if err != nil {
			b.Fatalf("Private decryption failed: %v", err)
		}

		totalDecryptTime := decryptPublicTime + decryptPrivateTime
		decryptTimings = append(decryptTimings, totalDecryptTime)
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
