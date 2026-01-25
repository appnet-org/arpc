package main

// Note: This file uses package main because it needs to be runnable as a standalone test
// The benchmarks will be executed with: go test -bench=. ./benchmark/encryption
//
// Triple Benchmark Pattern:
// - Whole: encrypt and decrypt 3 times, latency is sum of all three
// - Split: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split 2 times, latency is sum of these

import (
	"crypto/rand"
	"testing"
)

// BenchmarkTripleEncryption_Whole measures optimized encryption for whole strings
// with 3x encrypt/decrypt operations per iteration
func BenchmarkTripleEncryption_Whole(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
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

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Perform encrypt/decrypt 3 times and sum the latencies
		for round := 0; round < 3; round++ {
			// Encrypt with optimized function
			encrypted, encryptTime, err := encryptWholeWithTiming(data, true)
			if err != nil {
				b.Fatalf("Encryption failed: %v", err)
			}
			totalEncryptTime += encryptTime

			// Decrypt with optimized function
			_, decryptTime, err := decryptWholeWithTiming(encrypted, true)
			if err != nil {
				b.Fatalf("Decryption failed: %v", err)
			}
			totalDecryptTime += decryptTime
		}

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("triple_encryption_whole_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("triple_encryption_whole_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkTripleEncryption_RandomSplit measures optimized encryption for randomly split strings
// Pattern: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split 2 times
func BenchmarkTripleEncryption_RandomSplit(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
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

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the first split (public part) 2 more times
		for round := 0; round < 2; round++ {
			encrypted, encTime, err := encryptWholeWithTiming(publicPart, true)
			if err != nil {
				b.Fatalf("Encryption of first split failed: %v", err)
			}
			totalEncryptTime += encTime

			_, decTime, err := decryptWholeWithTiming(encrypted, true)
			if err != nil {
				b.Fatalf("Decryption of first split failed: %v", err)
			}
			totalDecryptTime += decTime
		}

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("triple_encryption_random_split_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("triple_encryption_random_split_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkTripleEncryption_EqualSplit measures optimized encryption for data split in half
// Pattern: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split 2 times
func BenchmarkTripleEncryption_EqualSplit(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
	traceSize := len(traceEntries)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		size := entry.TotalSize

		if size <= 1 {
			continue // Need at least 2 bytes to split
		}

		// Generate test data and split in half (excluded from timing)
		b.StopTimer()
		data := generateRandomBytes(size)
		splitPoint := size / 2
		publicPart := data[:splitPoint]
		privatePart := data[splitPoint:]
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		encryptedPublic, encryptedPrivate, encryptTime, err := encryptSplitWithTiming(publicPart, privatePart)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedPublic, encryptedPrivate)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the first split (public part) 2 more times
		for round := 0; round < 2; round++ {
			encrypted, encTime, err := encryptWholeWithTiming(publicPart, true)
			if err != nil {
				b.Fatalf("Encryption of first split failed: %v", err)
			}
			totalEncryptTime += encTime

			_, decTime, err := decryptWholeWithTiming(encrypted, true)
			if err != nil {
				b.Fatalf("Decryption of first split failed: %v", err)
			}
			totalDecryptTime += decTime
		}

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: size})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: size})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("triple_encryption_equal_split_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("triple_encryption_equal_split_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}

// BenchmarkTripleEncryption_KeyValueSplit measures optimized encryption for key/value split
// where the key is in one segment (public) and the value is in another segment (private)
// Pattern: encrypt/decrypt both splits 1 time + encrypt/decrypt only first split (key) 2 times
func BenchmarkTripleEncryption_KeyValueSplit(b *testing.B) {
	encryptTimings := make([]TimingEntry, 0, b.N)
	decryptTimings := make([]TimingEntry, 0, b.N)
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

		totalSize := keySize + valueSize

		// Generate test data for key and value separately (excluded from timing)
		b.StopTimer()
		keyData := generateRandomBytes(keySize)
		valueData := generateRandomBytes(valueSize)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		// Key as public part, Value as private part
		encryptedKey, encryptedValue, encryptTime, err := encryptSplitWithTiming(keyData, valueData)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedKey, encryptedValue)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the first split (key) 2 more times
		for round := 0; round < 2; round++ {
			encrypted, encTime, err := encryptWholeWithTiming(keyData, true)
			if err != nil {
				b.Fatalf("Encryption of key failed: %v", err)
			}
			totalEncryptTime += encTime

			_, decTime, err := decryptWholeWithTiming(encrypted, true)
			if err != nil {
				b.Fatalf("Decryption of key failed: %v", err)
			}
			totalDecryptTime += decTime
		}

		encryptTimings = append(encryptTimings, TimingEntry{LatencyNs: totalEncryptTime, MessageSize: totalSize})
		decryptTimings = append(decryptTimings, TimingEntry{LatencyNs: totalDecryptTime, MessageSize: totalSize})
	}

	b.StopTimer()

	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}

	if err := writeTimingsWithSize("triple_encryption_key_value_split_encrypt_times.csv", encryptTimings); err != nil {
		b.Logf("Failed to write encryption timing data: %v", err)
	}
	if err := writeTimingsWithSize("triple_encryption_key_value_split_decrypt_times.csv", decryptTimings); err != nil {
		b.Logf("Failed to write decryption timing data: %v", err)
	}

	b.StartTimer()
}
