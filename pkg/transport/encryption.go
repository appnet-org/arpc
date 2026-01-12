package transport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

// Default encryption keys (AES-256) - hardcoded for development/testing
// In production, these should be replaced with proper key management
var (
	// DefaultPublicKey is a hardcoded 32-byte key for encrypting public segments
	DefaultPublicKey, _ = hex.DecodeString("27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796")
	// DefaultPrivateKey is a hardcoded 32-byte key for encrypting private segments
	DefaultPrivateKey, _ = hex.DecodeString("9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9")
)

// Cached GCM objects (thread-safe)
var (
	publicGCM  cipher.AEAD
	privateGCM cipher.AEAD
	gcmInitMu  sync.RWMutex
)

// Pool for double nonces (24 bytes = 2 * 12) to reduce allocations
var doubleNoncePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 24)
		return &b
	},
}

// InitGCMObjects initializes cached GCM objects for the given public and private keys.
// This should be called when encryption keys are set to avoid recreating cipher and GCM objects.
func InitGCMObjects(publicKey, privateKey []byte) error {
	gcmInitMu.Lock()
	defer gcmInitMu.Unlock()

	var err error

	// Create AES cipher for public key
	publicBlock, err := aes.NewCipher(publicKey)
	if err != nil {
		return fmt.Errorf("failed to create public cipher: %w", err)
	}

	// Create GCM mode for public key
	publicGCM, err = cipher.NewGCM(publicBlock)
	if err != nil {
		return fmt.Errorf("failed to create public GCM: %w", err)
	}

	// Create AES cipher for private key
	privateBlock, err := aes.NewCipher(privateKey)
	if err != nil {
		return fmt.Errorf("failed to create private cipher: %w", err)
	}

	// Create GCM mode for private key
	privateGCM, err = cipher.NewGCM(privateBlock)
	if err != nil {
		return fmt.Errorf("failed to create private GCM: %w", err)
	}

	return nil
}

// EncryptSymphonyData encrypts Symphony marshaled data using AES-GCM.
// The public segment (bytes 13 to offsetToPrivate) is encrypted with publicKey.
// The private segment (bytes offsetToPrivate onwards, including version byte) is encrypted with privateKey.
//
// Parameters:
//   - data: Raw Symphony marshaled data
//   - publicKey: Symmetric key for encrypting public segment (required if public segment exists)
//   - privateKey: Symmetric key for encrypting private segment (required if private segment exists)
//
// Returns encrypted data with updated offsetToPrivate, or panics on error.
func EncryptSymphonyData(data []byte, publicKey []byte, privateKey []byte) []byte {
	// Validate minimum size
	if len(data) < 13 {
		panic("invalid Symphony data: too short for header")
	}

	// Parse offsetToPrivate from bytes [1:5]
	// Valid values: 13 <= offsetToPrivate <= len(data)
	// - offsetToPrivate == len(data) means public-only segment (no private segment)
	// - offsetToPrivate < len(data) means both public and private segments exist
	offsetToPrivate := int(binary.LittleEndian.Uint32(data[1:5]))
	if offsetToPrivate < 13 || offsetToPrivate > len(data) {
		panic(fmt.Sprintf("invalid offsetToPrivate: %d (data length: %d)", offsetToPrivate, len(data)))
	}

	// Validate publicKey is provided
	if publicKey == nil {
		panic("publicKey is required for encrypting public segment")
	}

	// Extract segments
	publicPlaintext := data[13:offsetToPrivate]
	hasPrivateSegment := offsetToPrivate < len(data)

	var encryptedPublic, encryptedPrivate []byte
	var err error

	if hasPrivateSegment {
		// Validate privateKey is provided
		if privateKey == nil {
			panic("privateKey is required for encrypting private segment")
		}

		// Batch nonce generation: get both nonces in a single rand.Read call
		noncesBuf := doubleNoncePool.Get().(*[]byte)
		nonces := *noncesBuf
		if _, err := rand.Read(nonces); err != nil {
			doubleNoncePool.Put(noncesBuf)
			panic(fmt.Sprintf("failed to generate nonces: %v", err))
		}

		// Encrypt public segment with first nonce
		encryptedPublic, err = encryptSegmentWithNonce(publicPlaintext, true, nonces[:12])
		if err != nil {
			doubleNoncePool.Put(noncesBuf)
			panic(fmt.Sprintf("failed to encrypt public segment: %v", err))
		}

		// Encrypt private segment with second nonce
		privatePlaintext := data[offsetToPrivate:]
		encryptedPrivate, err = encryptSegmentWithNonce(privatePlaintext, false, nonces[12:])
		if err != nil {
			doubleNoncePool.Put(noncesBuf)
			panic(fmt.Sprintf("failed to encrypt private segment: %v", err))
		}

		// Return pooled buffer
		doubleNoncePool.Put(noncesBuf)
	} else {
		// Public segment only - use standard encryption
		encryptedPublic, err = encryptSegment(publicPlaintext, true)
		if err != nil {
			panic(fmt.Sprintf("failed to encrypt public segment: %v", err))
		}
	}

	// Calculate new offsetToPrivate
	// New offset = 13 (header) + len(encryptedPublic)
	newOffsetToPrivate := 13 + len(encryptedPublic)

	// Reconstruct buffer: [header(13)][encrypted_public][encrypted_private]
	totalSize := 13 + len(encryptedPublic) + len(encryptedPrivate)
	result := make([]byte, totalSize)

	// Copy header (first 13 bytes)
	copy(result[0:13], data[0:13])

	// Update offsetToPrivate in header
	binary.LittleEndian.PutUint32(result[1:5], uint32(newOffsetToPrivate))

	// Copy encrypted public segment
	copy(result[13:], encryptedPublic)

	// Copy encrypted private segment if exists
	if hasPrivateSegment {
		copy(result[newOffsetToPrivate:], encryptedPrivate)
	}

	return result
}

// DecryptSymphonyData decrypts Symphony encrypted data using AES-GCM.
// The public segment is decrypted with publicKey.
// The private segment (if exists) is decrypted with privateKey.
//
// Parameters:
//   - data: Encrypted Symphony data
//   - publicKey: Symmetric key for decrypting public segment (required)
//   - privateKey: Symmetric key for decrypting private segment (required if private segment exists)
//
// Returns decrypted data with original offsetToPrivate, or panics on error.
func DecryptSymphonyData(data []byte, publicKey []byte, privateKey []byte) []byte {
	// Validate minimum size
	if len(data) < 13 {
		panic("invalid encrypted data: too short for header")
	}

	// Validate publicKey is provided
	if publicKey == nil {
		panic("publicKey is required for decrypting public segment")
	}

	// Parse offsetToPrivate from bytes [1:5]
	// Valid values: (13+28) <= encryptedOffsetToPrivate <= len(data)
	// - Minimum 13+28 because encrypted public segment needs 12 bytes nonce + 16 bytes tag
	// - encryptedOffsetToPrivate == len(data) means public-only (no private segment)
	// - encryptedOffsetToPrivate < len(data) means both segments exist
	encryptedOffsetToPrivate := int(binary.LittleEndian.Uint32(data[1:5]))
	if encryptedOffsetToPrivate < 13+28 || encryptedOffsetToPrivate > len(data) {
		panic(fmt.Sprintf("invalid encrypted offsetToPrivate: %d (data length: %d)", encryptedOffsetToPrivate, len(data)))
	}

	// Extract and decrypt public segment
	encryptedPublic := data[13:encryptedOffsetToPrivate]
	publicPlaintext, err := decryptSegment(encryptedPublic, true)
	if err != nil {
		panic(fmt.Sprintf("failed to decrypt public segment: %v", err))
	}

	// Calculate original offsetToPrivate
	originalOffsetToPrivate := 13 + len(publicPlaintext)

	// Check if private segment exists
	hasPrivateSegment := encryptedOffsetToPrivate < len(data)
	var privatePlaintext []byte

	if hasPrivateSegment {
		// Validate privateKey is provided
		if privateKey == nil {
			panic("privateKey is required for decrypting private segment")
		}

		// Extract and decrypt private segment
		encryptedPrivate := data[encryptedOffsetToPrivate:]
		privatePlaintext, err = decryptSegment(encryptedPrivate, false)
		if err != nil {
			panic(fmt.Sprintf("failed to decrypt private segment: %v", err))
		}

		// Validate that decrypted private segment starts with version byte 0x01
		if len(privatePlaintext) < 1 || privatePlaintext[0] != 0x01 {
			panic("invalid decrypted private segment: missing or incorrect version byte")
		}
	}

	// Reconstruct decrypted buffer
	totalSize := 13 + len(publicPlaintext) + len(privatePlaintext)
	result := make([]byte, totalSize)

	// Copy header (first 13 bytes)
	copy(result[0:13], data[0:13])

	// Update offsetToPrivate to original value
	binary.LittleEndian.PutUint32(result[1:5], uint32(originalOffsetToPrivate))

	// Copy decrypted public segment
	copy(result[13:], publicPlaintext)

	// Copy decrypted private segment if exists
	if hasPrivateSegment {
		copy(result[originalOffsetToPrivate:], privatePlaintext)
	}

	return result
}

// encryptSegment encrypts plaintext using AES-GCM.
// Returns [nonce(12 bytes)][ciphertext+tag(16 bytes)].
// Note: Nonce is NOT reusable - each encryption must use a unique nonce for security.
// isPublic: true to use public key GCM, false to use private key GCM
func encryptSegment(plaintext []byte, isPublic bool) ([]byte, error) {
	return encryptSegmentWithNonce(plaintext, isPublic, nil)
}

// encryptSegmentWithNonce encrypts plaintext using AES-GCM with an optional pre-generated nonce.
// If nonce is nil, a new random nonce is generated.
// Returns [nonce(12 bytes)][ciphertext+tag(16 bytes)].
func encryptSegmentWithNonce(plaintext []byte, isPublic bool, nonce []byte) ([]byte, error) {
	// Get cached GCM object (reuses cipher and GCM objects)
	var gcm cipher.AEAD
	if isPublic {
		gcm = publicGCM
	} else {
		gcm = privateGCM
	}

	nonceSize := gcm.NonceSize()
	tagSize := gcm.Overhead()

	// Generate nonce if not provided
	if nonce == nil {
		nonce = make([]byte, nonceSize)
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("failed to generate nonce: %w", err)
		}
	}

	// Pre-allocate output buffer with exact capacity: nonce + plaintext + tag
	totalSize := nonceSize + len(plaintext) + tagSize
	result := make([]byte, nonceSize, totalSize)
	copy(result, nonce)

	// Encrypt and authenticate, appending to pre-allocated buffer
	result = gcm.Seal(result, nonce, plaintext, nil)

	return result, nil
}

// decryptSegment decrypts encrypted data using AES-GCM.
// Expects input format: [nonce(12 bytes)][ciphertext+tag(16 bytes)].
// Returns plaintext or error if authentication fails.
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
	tagSize := gcm.Overhead()
	if len(encrypted) < nonceSize+tagSize {
		return nil, fmt.Errorf("encrypted data too short: %d bytes (expected at least %d)", len(encrypted), nonceSize+tagSize)
	}

	// Extract nonce and ciphertext+tag
	nonce := encrypted[:nonceSize]
	ciphertextWithTag := encrypted[nonceSize:]

	// Pre-allocate output buffer with exact capacity
	plaintextSize := len(ciphertextWithTag) - tagSize
	result := make([]byte, 0, plaintextSize)

	// Decrypt and authenticate
	plaintext, err := gcm.Open(result, nonce, ciphertextWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
