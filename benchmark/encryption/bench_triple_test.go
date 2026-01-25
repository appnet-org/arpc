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

		// Generate fresh random data for each round (excluded from timing)
		// This avoids cache advantages from reusing the same data
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

		// Generate fresh random data for each operation (excluded from timing)
		// This avoids cache advantages from reusing the same data
		b.StopTimer()
		// Data for Step 1: full split operation
		data1 := generateRandomBytes(size)
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
		publicPart1 := data1[:splitPoint]
		privatePart1 := data1[splitPoint:]

		// Data for Step 2: first split only (round 1)
		firstSplitData2 := generateRandomBytes(splitPoint)

		// Data for Step 3: first split only (round 2)
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

		// Step 2: Encrypt/decrypt only the first split (fresh data)
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

		// Step 3: Encrypt/decrypt only the first split (fresh data)
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

		// Generate fresh random data for each operation (excluded from timing)
		// This avoids cache advantages from reusing the same data
		b.StopTimer()
		splitPoint := size / 2

		// Data for Step 1: full split operation
		data1 := generateRandomBytes(size)
		publicPart1 := data1[:splitPoint]
		privatePart1 := data1[splitPoint:]

		// Data for Step 2: first split only (round 1)
		firstSplitData2 := generateRandomBytes(splitPoint)

		// Data for Step 3: first split only (round 2)
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

		// Step 2: Encrypt/decrypt only the first split (fresh data)
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

		// Step 3: Encrypt/decrypt only the first split (fresh data)
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

		// Generate fresh random data for each operation (excluded from timing)
		// This avoids cache advantages from reusing the same data
		b.StopTimer()
		// Data for Step 1: full split operation (key + value)
		keyData1 := generateRandomBytes(keySize)
		valueData1 := generateRandomBytes(valueSize)

		// Data for Step 2: key only (round 1)
		keyData2 := generateRandomBytes(keySize)

		// Data for Step 3: key only (round 2)
		keyData3 := generateRandomBytes(keySize)
		b.StartTimer()

		var totalEncryptTime int64
		var totalDecryptTime int64

		// Step 1: Encrypt/decrypt both splits once
		// Key as public part, Value as private part
		encryptedKey, encryptedValue, encryptTime, err := encryptSplitWithTiming(keyData1, valueData1)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
		totalEncryptTime += encryptTime

		_, _, decryptTime, err := decryptSplitWithTiming(encryptedKey, encryptedValue)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
		totalDecryptTime += decryptTime

		// Step 2: Encrypt/decrypt only the key (fresh data)
		encrypted2, encTime2, err := encryptWholeWithTiming(keyData2, true)
		if err != nil {
			b.Fatalf("Encryption of key failed: %v", err)
		}
		totalEncryptTime += encTime2

		_, decTime2, err := decryptWholeWithTiming(encrypted2, true)
		if err != nil {
			b.Fatalf("Decryption of key failed: %v", err)
		}
		totalDecryptTime += decTime2

		// Step 3: Encrypt/decrypt only the key (fresh data)
		encrypted3, encTime3, err := encryptWholeWithTiming(keyData3, true)
		if err != nil {
			b.Fatalf("Encryption of key failed: %v", err)
		}
		totalEncryptTime += encTime3

		_, decTime3, err := decryptWholeWithTiming(encrypted3, true)
		if err != nil {
			b.Fatalf("Decryption of key failed: %v", err)
		}
		totalDecryptTime += decTime3

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
