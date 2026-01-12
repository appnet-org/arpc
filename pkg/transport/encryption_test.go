package transport

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
)

// --- Test Helpers ---

// createSymphonyData creates valid Symphony marshaled test data with the specified
// public and private segment sizes.
// Format: [version(1)][offsetToPrivate(4)][serviceID(4)][methodID(4)][public...][0x01][private...]
func createSymphonyData(publicSize, privateSize int) []byte {
	// Header is always 13 bytes: version(1) + offsetToPrivate(4) + serviceID(4) + methodID(4)
	offsetToPrivate := 13 + publicSize
	totalSize := offsetToPrivate
	if privateSize > 0 {
		totalSize += 1 + privateSize // +1 for version byte in private segment
	}

	data := make([]byte, totalSize)
	data[0] = 0x01 // version
	binary.LittleEndian.PutUint32(data[1:5], uint32(offsetToPrivate))
	binary.LittleEndian.PutUint32(data[5:9], 0)  // serviceID
	binary.LittleEndian.PutUint32(data[9:13], 0) // methodID

	// Fill public segment with pattern
	for i := 13; i < offsetToPrivate; i++ {
		data[i] = byte(i % 256)
	}

	// Fill private segment if exists
	if privateSize > 0 {
		data[offsetToPrivate] = 0x01 // version byte required for private segment
		for i := offsetToPrivate + 1; i < totalSize; i++ {
			data[i] = byte((i * 2) % 256)
		}
	}

	return data
}

// assertPanic verifies that the given function panics with a message containing expected substring
func assertPanic(t *testing.T, name string, expectedSubstr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: expected panic but none occurred", name)
			return
		}
		msg := fmt.Sprintf("%v", r)
		if expectedSubstr != "" && !bytes.Contains([]byte(msg), []byte(expectedSubstr)) {
			t.Errorf("%s: panic message %q does not contain %q", name, msg, expectedSubstr)
		}
	}()
	fn()
}

// --- InitGCMObjects Tests ---

func TestInitGCMObjects(t *testing.T) {
	t.Run("ValidAES256Keys", func(t *testing.T) {
		publicKey, _ := hex.DecodeString("27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796")
		privateKey, _ := hex.DecodeString("9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9")

		err := InitGCMObjects(publicKey, privateKey)
		if err != nil {
			t.Fatalf("InitGCMObjects failed with valid keys: %v", err)
		}
	})

	t.Run("ValidAES128Keys", func(t *testing.T) {
		// AES supports 16, 24, and 32 byte keys
		key16 := make([]byte, 16)
		for i := range key16 {
			key16[i] = byte(i)
		}

		err := InitGCMObjects(key16, key16)
		if err != nil {
			t.Fatalf("InitGCMObjects failed with 16-byte keys: %v", err)
		}
	})

	t.Run("ValidAES192Keys", func(t *testing.T) {
		key24 := make([]byte, 24)
		for i := range key24 {
			key24[i] = byte(i)
		}

		err := InitGCMObjects(key24, key24)
		if err != nil {
			t.Fatalf("InitGCMObjects failed with 24-byte keys: %v", err)
		}
	})

	t.Run("InvalidKeySize", func(t *testing.T) {
		invalidKey := make([]byte, 15) // Invalid size
		validKey := make([]byte, 32)

		err := InitGCMObjects(invalidKey, validKey)
		if err == nil {
			t.Error("Expected error for invalid public key size")
		}

		err = InitGCMObjects(validKey, invalidKey)
		if err == nil {
			t.Error("Expected error for invalid private key size")
		}
	})

	t.Run("EmptyKeys", func(t *testing.T) {
		err := InitGCMObjects([]byte{}, []byte{})
		if err == nil {
			t.Error("Expected error for empty keys")
		}
	})

	// Re-initialize with default keys for subsequent tests
	t.Cleanup(func() {
		_ = InitGCMObjects(DefaultPublicKey, DefaultPrivateKey)
	})
}

// --- EncryptSymphonyData Tests ---

func TestEncryptSymphonyData(t *testing.T) {
	// Ensure GCM objects are initialized
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("PublicSegmentOnly", func(t *testing.T) {
		testCases := []struct {
			name       string
			publicSize int
		}{
			{"Empty", 0},
			{"Small", 10},
			{"Medium", 100},
			{"Large", 1000},
			{"VeryLarge", 10000},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				original := createSymphonyData(tc.publicSize, 0)
				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)

				// Encrypted data should be larger (nonce + tag overhead)
				expectedMinSize := 13 + tc.publicSize + 12 + 16 // header + public + nonce + tag
				if len(encrypted) < expectedMinSize {
					t.Errorf("Encrypted size %d < expected min %d", len(encrypted), expectedMinSize)
				}

				// Verify header version is preserved
				if encrypted[0] != original[0] {
					t.Errorf("Version byte changed: got %d, want %d", encrypted[0], original[0])
				}

				// Verify offsetToPrivate was updated
				newOffset := int(binary.LittleEndian.Uint32(encrypted[1:5]))
				if newOffset != len(encrypted) {
					t.Errorf("offsetToPrivate should equal len(encrypted) for public-only: got %d, want %d",
						newOffset, len(encrypted))
				}
			})
		}
	})

	t.Run("BothSegments", func(t *testing.T) {
		testCases := []struct {
			name        string
			publicSize  int
			privateSize int
		}{
			{"SmallBoth", 10, 10},
			{"MediumBoth", 100, 100},
			{"LargeBoth", 1000, 1000},
			{"LargePublicSmallPrivate", 1000, 10},
			{"SmallPublicLargePrivate", 10, 1000},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				original := createSymphonyData(tc.publicSize, tc.privateSize)
				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)

				// Encrypted data should be larger
				// Overhead: 2 * (nonce + tag) = 2 * (12 + 16) = 56 bytes
				minOverhead := 56
				if len(encrypted) < len(original)+minOverhead {
					t.Errorf("Encrypted size %d < original %d + overhead %d",
						len(encrypted), len(original), minOverhead)
				}

				// Verify offsetToPrivate points to private segment
				newOffset := int(binary.LittleEndian.Uint32(encrypted[1:5]))
				if newOffset >= len(encrypted) {
					t.Errorf("offsetToPrivate %d should be < len(encrypted) %d for two-segment data",
						newOffset, len(encrypted))
				}
			})
		}
	})

	t.Run("HeaderPreserved", func(t *testing.T) {
		original := createSymphonyData(100, 50)
		// Set custom service/method IDs
		binary.LittleEndian.PutUint32(original[5:9], 12345)  // serviceID
		binary.LittleEndian.PutUint32(original[9:13], 67890) // methodID

		encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)

		// Service and method IDs should be preserved
		encServiceID := binary.LittleEndian.Uint32(encrypted[5:9])
		encMethodID := binary.LittleEndian.Uint32(encrypted[9:13])

		if encServiceID != 12345 {
			t.Errorf("ServiceID not preserved: got %d, want 12345", encServiceID)
		}
		if encMethodID != 67890 {
			t.Errorf("MethodID not preserved: got %d, want 67890", encMethodID)
		}
	})
}

func TestEncryptSymphonyData_Panics(t *testing.T) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("DataTooShort", func(t *testing.T) {
		assertPanic(t, "10-byte data", "too short", func() {
			EncryptSymphonyData(make([]byte, 10), DefaultPublicKey, DefaultPrivateKey)
		})

		assertPanic(t, "0-byte data", "too short", func() {
			EncryptSymphonyData([]byte{}, DefaultPublicKey, DefaultPrivateKey)
		})

		assertPanic(t, "12-byte data", "too short", func() {
			EncryptSymphonyData(make([]byte, 12), DefaultPublicKey, DefaultPrivateKey)
		})
	})

	t.Run("NilPublicKey", func(t *testing.T) {
		assertPanic(t, "nil public key", "publicKey is required", func() {
			data := createSymphonyData(10, 0)
			EncryptSymphonyData(data, nil, DefaultPrivateKey)
		})
	})

	t.Run("NilPrivateKeyWithPrivateSegment", func(t *testing.T) {
		assertPanic(t, "nil private key with private segment", "privateKey is required", func() {
			data := createSymphonyData(10, 10)
			EncryptSymphonyData(data, DefaultPublicKey, nil)
		})
	})

	t.Run("InvalidOffsetToPrivate", func(t *testing.T) {
		// Offset < 13
		assertPanic(t, "offset < 13", "invalid offsetToPrivate", func() {
			data := make([]byte, 20)
			binary.LittleEndian.PutUint32(data[1:5], 5)
			EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		})

		// Offset > len(data)
		assertPanic(t, "offset > len", "invalid offsetToPrivate", func() {
			data := make([]byte, 20)
			binary.LittleEndian.PutUint32(data[1:5], 100)
			EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		})
	})
}

// --- DecryptSymphonyData Tests ---

func TestDecryptSymphonyData(t *testing.T) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("RoundTrip_PublicOnly", func(t *testing.T) {
		testCases := []int{0, 10, 100, 1000, 10000}
		for _, size := range testCases {
			t.Run(fmt.Sprintf("Size%d", size), func(t *testing.T) {
				original := createSymphonyData(size, 0)
				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
				decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

				if !bytes.Equal(original, decrypted) {
					t.Errorf("Round-trip failed for public size %d", size)
					if len(original) != len(decrypted) {
						t.Errorf("Length mismatch: original %d, decrypted %d", len(original), len(decrypted))
					}
				}
			})
		}
	})

	t.Run("RoundTrip_BothSegments", func(t *testing.T) {
		testCases := []struct {
			publicSize  int
			privateSize int
		}{
			{10, 10},
			{100, 100},
			{1000, 1000},
			{10, 1000},
			{1000, 10},
			{5000, 5000},
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("Pub%d_Priv%d", tc.publicSize, tc.privateSize), func(t *testing.T) {
				original := createSymphonyData(tc.publicSize, tc.privateSize)
				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
				decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

				if !bytes.Equal(original, decrypted) {
					t.Errorf("Round-trip failed")
					if len(original) != len(decrypted) {
						t.Errorf("Length mismatch: original %d, decrypted %d", len(original), len(decrypted))
					}
				}
			})
		}
	})

	t.Run("OffsetRestored", func(t *testing.T) {
		original := createSymphonyData(200, 100)
		originalOffset := binary.LittleEndian.Uint32(original[1:5])

		encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
		decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

		restoredOffset := binary.LittleEndian.Uint32(decrypted[1:5])
		if restoredOffset != originalOffset {
			t.Errorf("Offset not restored: original %d, restored %d", originalOffset, restoredOffset)
		}
	})
}

func TestDecryptSymphonyData_Panics(t *testing.T) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("DataTooShort", func(t *testing.T) {
		assertPanic(t, "short data", "too short", func() {
			DecryptSymphonyData(make([]byte, 10), DefaultPublicKey, DefaultPrivateKey)
		})
	})

	t.Run("NilPublicKey", func(t *testing.T) {
		assertPanic(t, "nil public key", "publicKey is required", func() {
			original := createSymphonyData(10, 0)
			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			DecryptSymphonyData(encrypted, nil, DefaultPrivateKey)
		})
	})

	t.Run("NilPrivateKeyWithPrivateSegment", func(t *testing.T) {
		assertPanic(t, "nil private key", "privateKey is required", func() {
			original := createSymphonyData(10, 10)
			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			DecryptSymphonyData(encrypted, DefaultPublicKey, nil)
		})
	})

	t.Run("TamperedCiphertext", func(t *testing.T) {
		assertPanic(t, "tampered public segment", "decrypt", func() {
			original := createSymphonyData(100, 0)
			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			// Tamper with encrypted public segment
			encrypted[20] ^= 0xFF
			DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)
		})
	})

	t.Run("TamperedPrivateSegment", func(t *testing.T) {
		assertPanic(t, "tampered private segment", "decrypt", func() {
			original := createSymphonyData(10, 100)
			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			// Tamper with encrypted private segment (near end)
			encrypted[len(encrypted)-5] ^= 0xFF
			DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)
		})
	})

	t.Run("InvalidEncryptedOffset", func(t *testing.T) {
		assertPanic(t, "invalid offset", "invalid encrypted offsetToPrivate", func() {
			data := make([]byte, 50)
			data[0] = 0x01
			// Set offset too small (less than 13+28)
			binary.LittleEndian.PutUint32(data[1:5], 20)
			DecryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		})
	})

	t.Run("MissingPrivateVersionByte", func(t *testing.T) {
		assertPanic(t, "wrong version byte", "version byte", func() {
			// Create valid data but with wrong private segment version byte
			original := createSymphonyData(10, 10)
			// Change private version byte to invalid value
			offsetToPrivate := int(binary.LittleEndian.Uint32(original[1:5]))
			original[offsetToPrivate] = 0x00 // Invalid version

			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)
		})
	})
}

// --- Concurrent Access Tests ---

func TestEncryptDecrypt_Concurrent(t *testing.T) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("ConcurrentEncryptDecrypt", func(t *testing.T) {
		const numGoroutines = 100
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()

				// Create unique data for each goroutine
				publicSize := 50 + (idx % 100)
				privateSize := 50 + ((idx * 3) % 100)
				original := createSymphonyData(publicSize, privateSize)

				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
				decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

				if !bytes.Equal(original, decrypted) {
					errors <- fmt.Errorf("goroutine %d: round-trip mismatch (pub=%d, priv=%d)",
						idx, publicSize, privateSize)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
	})

	t.Run("ConcurrentPublicOnly", func(t *testing.T) {
		const numGoroutines = 50
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()

				size := 100 + (idx * 10)
				original := createSymphonyData(size, 0)

				encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
				decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

				if !bytes.Equal(original, decrypted) {
					errors <- fmt.Errorf("goroutine %d: public-only round-trip mismatch (size=%d)",
						idx, size)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
	})
}

func TestInitGCMObjects_CalledOnce(t *testing.T) {
	// In production, InitGCMObjects is called once at startup.
	// This test verifies that after initialization, concurrent encrypt/decrypt works.

	// Initialize once
	err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey)
	if err != nil {
		t.Fatalf("InitGCMObjects failed: %v", err)
	}

	// Then test concurrent operations
	const numGoroutines = 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			original := createSymphonyData(50+idx, 50+idx)
			encrypted := EncryptSymphonyData(original, DefaultPublicKey, DefaultPrivateKey)
			decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

			if !bytes.Equal(original, decrypted) {
				errors <- fmt.Errorf("goroutine %d: round-trip failed", idx)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// --- Edge Cases ---

func TestEdgeCases(t *testing.T) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		t.Fatalf("Failed to init GCM objects: %v", err)
	}

	t.Run("MinimumValidData", func(t *testing.T) {
		// Minimum valid data is 13 bytes (header only, empty public segment)
		data := createSymphonyData(0, 0)
		if len(data) != 13 {
			t.Fatalf("Minimum data should be 13 bytes, got %d", len(data))
		}

		encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

		if !bytes.Equal(data, decrypted) {
			t.Error("Minimum data round-trip failed")
		}
	})

	t.Run("ExactlyOneBytePublic", func(t *testing.T) {
		data := createSymphonyData(1, 0)
		encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

		if !bytes.Equal(data, decrypted) {
			t.Error("1-byte public segment round-trip failed")
		}
	})

	t.Run("ExactlyOneBytePrivate", func(t *testing.T) {
		// Private segment: 1 byte version + 1 byte data
		data := createSymphonyData(0, 1)
		encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

		if !bytes.Equal(data, decrypted) {
			t.Error("1-byte private segment round-trip failed")
		}
	})

	t.Run("LargeData", func(t *testing.T) {
		// Test with ~1MB of data
		data := createSymphonyData(500000, 500000)
		encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		decrypted := DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)

		if !bytes.Equal(data, decrypted) {
			t.Error("Large data round-trip failed")
		}
	})

	t.Run("EncryptionIsNonDeterministic", func(t *testing.T) {
		// Due to random nonces, encrypting same data twice should produce different ciphertext
		data := createSymphonyData(100, 100)

		encrypted1 := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		encrypted2 := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)

		if bytes.Equal(encrypted1, encrypted2) {
			t.Error("Encryption should be non-deterministic (random nonces)")
		}

		// But both should decrypt to same plaintext
		decrypted1 := DecryptSymphonyData(encrypted1, DefaultPublicKey, DefaultPrivateKey)
		decrypted2 := DecryptSymphonyData(encrypted2, DefaultPublicKey, DefaultPrivateKey)

		if !bytes.Equal(decrypted1, decrypted2) {
			t.Error("Different encryptions should decrypt to same data")
		}
	})
}

// --- Benchmark Tests ---

func BenchmarkEncryptSymphonyData(b *testing.B) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		b.Fatalf("Failed to init GCM objects: %v", err)
	}

	benchCases := []struct {
		name        string
		publicSize  int
		privateSize int
	}{
		{"Small_100B", 50, 50},
		{"Medium_1KB", 500, 500},
		{"Large_10KB", 5000, 5000},
		{"Large_100KB", 50000, 50000},
		{"PublicOnly_1KB", 1000, 0},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			data := createSymphonyData(bc.publicSize, bc.privateSize)
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
			}
		})
	}
}

func BenchmarkDecryptSymphonyData(b *testing.B) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		b.Fatalf("Failed to init GCM objects: %v", err)
	}

	benchCases := []struct {
		name        string
		publicSize  int
		privateSize int
	}{
		{"Small_100B", 50, 50},
		{"Medium_1KB", 500, 500},
		{"Large_10KB", 5000, 5000},
		{"Large_100KB", 50000, 50000},
		{"PublicOnly_1KB", 1000, 0},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			data := createSymphonyData(bc.publicSize, bc.privateSize)
			encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)
			}
		})
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	if err := InitGCMObjects(DefaultPublicKey, DefaultPrivateKey); err != nil {
		b.Fatalf("Failed to init GCM objects: %v", err)
	}

	data := createSymphonyData(500, 500) // 1KB total
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		encrypted := EncryptSymphonyData(data, DefaultPublicKey, DefaultPrivateKey)
		_ = DecryptSymphonyData(encrypted, DefaultPublicKey, DefaultPrivateKey)
	}
}
