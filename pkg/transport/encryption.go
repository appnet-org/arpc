package transport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Default encryption keys (AES-256) - hardcoded for development/testing
// In production, these should be replaced with proper key management
var (
	// DefaultPublicKey is a hardcoded 32-byte key for encrypting public segments
	DefaultPublicKey, _ = hex.DecodeString("27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796")
	// DefaultPrivateKey is a hardcoded 32-byte key for encrypting private segments
	DefaultPrivateKey, _ = hex.DecodeString("9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9")
)

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

	// Extract and encrypt public segment (bytes 13 to offsetToPrivate)
	publicPlaintext := data[13:offsetToPrivate]
	encryptedPublic, err := encryptSegment(publicPlaintext, publicKey)
	if err != nil {
		panic(fmt.Sprintf("failed to encrypt public segment: %v", err))
	}

	// Calculate new offsetToPrivate
	// New offset = 13 (header) + len(encryptedPublic)
	newOffsetToPrivate := 13 + len(encryptedPublic)

	// Check if private segment exists
	hasPrivateSegment := offsetToPrivate < len(data)
	var encryptedPrivate []byte

	if hasPrivateSegment {
		// Validate privateKey is provided
		if privateKey == nil {
			panic("privateKey is required for encrypting private segment")
		}

		// Extract and encrypt entire private segment (including version byte)
		privatePlaintext := data[offsetToPrivate:]
		encryptedPrivate, err = encryptSegment(privatePlaintext, privateKey)
		if err != nil {
			panic(fmt.Sprintf("failed to encrypt private segment: %v", err))
		}
	}

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
	publicPlaintext, err := decryptSegment(encryptedPublic, publicKey)
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
		privatePlaintext, err = decryptSegment(encryptedPrivate, privateKey)
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

// encryptSegment encrypts plaintext using AES-GCM with the provided key.
// Returns [nonce(12 bytes)][ciphertext+tag(16 bytes)].
func encryptSegment(plaintext []byte, key []byte) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce (12 bytes for standard GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	// Seal appends the ciphertext and tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// decryptSegment decrypts encrypted data using AES-GCM with the provided key.
// Expects input format: [nonce(12 bytes)][ciphertext+tag(16 bytes)].
// Returns plaintext or error if authentication fails.
func decryptSegment(encrypted []byte, key []byte) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
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
