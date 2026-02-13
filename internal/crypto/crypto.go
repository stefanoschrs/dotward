package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/argon2"
)

const (
	magicHeader = "DOT1"
	versionByte = byte(1)
	saltSize    = 16
	keySize     = 32
)

var (
	currentArgon2 = argon2Params{
		time:    3,
		memory:  64 * 1024,
		threads: 4,
	}
	// legacyArgon2Profiles keep backwards compatibility for older encrypted files.
	legacyArgon2Profiles = []argon2Params{
		currentArgon2,
		{time: 1, memory: 64 * 1024, threads: 4},
	}
)

type argon2Params struct {
	time    uint32
	memory  uint32
	threads uint8
}

// EncryptFile encrypts src into dst using an Argon2id-derived AES-256-GCM key.
func EncryptFile(src, dst string, password []byte) error {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read plaintext file %q: %w", src, err)
	}
	defer zeroBytes(plaintext)

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	key := deriveKey(password, salt, currentArgon2)
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to initialize aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to initialize aes-gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	payload := make([]byte, 0, len(magicHeader)+1+len(salt)+len(nonce)+len(ciphertext))
	payload = append(payload, []byte(magicHeader)...)
	payload = append(payload, versionByte)
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)

	if err := os.WriteFile(dst, payload, 0o600); err != nil {
		return fmt.Errorf("failed to write encrypted file %q: %w", dst, err)
	}
	return nil
}

// DecryptFile decrypts src into dst using an Argon2id-derived AES-256-GCM key.
func DecryptFile(src, dst string, password []byte) error {
	payload, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file %q: %w", src, err)
	}

	if len(payload) < saltSize+12+16 {
		return errors.New("encrypted payload is too short")
	}

	var salt, nonce, ciphertext []byte
	if len(payload) >= len(magicHeader)+1 && string(payload[:len(magicHeader)]) == magicHeader {
		if payload[len(magicHeader)] != versionByte {
			return fmt.Errorf("unsupported encrypted file version: %d", payload[len(magicHeader)])
		}
		offset := len(magicHeader) + 1
		if len(payload) < offset+saltSize+12+16 {
			return errors.New("encrypted payload is too short")
		}
		salt = payload[offset : offset+saltSize]
		offset += saltSize
		nonce = payload[offset : offset+12]
		offset += 12
		ciphertext = payload[offset:]
	} else {
		// Legacy layout support: [salt|nonce|ciphertext] without header/version.
		salt = payload[:saltSize]
		nonce = payload[saltSize : saltSize+12]
		ciphertext = payload[saltSize+12:]
	}

	var lastErr error
	for _, params := range legacyArgon2Profiles {
		plaintext, err := decryptPayload(password, params, salt, nonce, ciphertext)
		if err != nil {
			lastErr = err
			continue
		}
		defer zeroBytes(plaintext)

		if err := os.WriteFile(dst, plaintext, 0o600); err != nil {
			return fmt.Errorf("failed to write plaintext file %q: %w", dst, err)
		}
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("failed to decrypt payload: %w", lastErr)
	}
	return errors.New("failed to decrypt payload")
}

func decryptPayload(password []byte, params argon2Params, salt, nonce, ciphertext []byte) ([]byte, error) {
	key := deriveKey(password, salt, params)
	defer zeroBytes(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aes-gcm: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce size")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func deriveKey(password, salt []byte, params argon2Params) []byte {
	pw := append([]byte(nil), password...)
	defer zeroBytes(pw)
	return argon2.IDKey(pw, salt, params.time, params.memory, params.threads, keySize)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
