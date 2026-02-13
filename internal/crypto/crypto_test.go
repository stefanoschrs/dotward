package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, "a.env")
	encPath := plainPath + ".enc"
	decPath := filepath.Join(dir, "out.env")

	in := []byte("API_KEY=abc123\n")
	if err := os.WriteFile(plainPath, in, 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	pwEnc := []byte("passphrase")
	if err := EncryptFile(plainPath, encPath, pwEnc); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	pwDec := []byte("passphrase")
	if err := DecryptFile(encPath, decPath, pwDec); err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	out, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("mismatch got=%q want=%q", string(out), string(in))
	}
}

func TestDecryptBadPasswordFails(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, "a.env")
	encPath := plainPath + ".enc"
	decPath := filepath.Join(dir, "out.env")

	if err := os.WriteFile(plainPath, []byte("X=1\n"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	pwEnc := []byte("right")
	if err := EncryptFile(plainPath, encPath, pwEnc); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	pwDec := []byte("wrong")
	if err := DecryptFile(encPath, decPath, pwDec); err == nil {
		t.Fatal("expected decrypt to fail with wrong password")
	}
}

func TestDecryptLegacyPayloadNoHeader(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, "a.env")
	encPath := plainPath + ".enc"
	decPath := filepath.Join(dir, "out.env")

	in := []byte("TOKEN=legacy-format\n")
	if err := os.WriteFile(plainPath, in, 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	pw := []byte("passphrase")
	if err := encryptLegacyNoHeader(plainPath, encPath, pw); err != nil {
		t.Fatalf("encrypt legacy: %v", err)
	}

	pwDec := []byte("passphrase")
	if err := DecryptFile(encPath, decPath, pwDec); err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}

	out, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("mismatch got=%q want=%q", string(out), string(in))
	}
}

func encryptLegacyNoHeader(src, dst string, password []byte) error {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	defer zeroBytes(plaintext)

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}

	legacy := argon2.IDKey(password, salt, 1, 64*1024, 4, keySize)
	defer zeroBytes(legacy)

	block, err := aes.NewCipher(legacy)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	payload := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	return os.WriteFile(dst, payload, 0o600)
}
