package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cryptopkg "github.com/stefanos/dotward/internal/crypto"
)

func TestUpdateOneFileRejectsWrongExistingPassword(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")
	encPath := plainPath + ".enc"

	if err := os.WriteFile(plainPath, []byte("TOKEN=old\n"), 0o600); err != nil {
		t.Fatalf("write initial plaintext: %v", err)
	}
	if err := cryptopkg.EncryptFile(plainPath, encPath, []byte("1234")); err != nil {
		t.Fatalf("encrypt initial file: %v", err)
	}
	before, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read encrypted file before update: %v", err)
	}

	if err := os.WriteFile(plainPath, []byte("TOKEN=new\n"), 0o600); err != nil {
		t.Fatalf("write updated plaintext: %v", err)
	}
	if _, err := updateOneFile(plainPath, []byte("12345")); err == nil {
		t.Fatal("expected update to reject the wrong existing password")
	}

	after, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read encrypted file after failed update: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("failed update changed the existing encrypted file")
	}

	plaintext, err := cryptopkg.Decrypt(encPath, []byte("1234"))
	if err != nil {
		t.Fatalf("decrypt with original password: %v", err)
	}
	defer zeroBytes(plaintext)
	if string(plaintext) != "TOKEN=old\n" {
		t.Fatalf("encrypted file changed got=%q want=%q", plaintext, "TOKEN=old\n")
	}
	if _, err := cryptopkg.Decrypt(encPath, []byte("12345")); err == nil {
		t.Fatal("encrypted file should not accept the wrong update password")
	}
}

func TestUpdateOneFileWithCorrectPasswordReencryptsPlaintext(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")
	encPath := plainPath + ".enc"

	if err := os.WriteFile(plainPath, []byte("TOKEN=old\n"), 0o600); err != nil {
		t.Fatalf("write initial plaintext: %v", err)
	}
	if err := cryptopkg.EncryptFile(plainPath, encPath, []byte("1234")); err != nil {
		t.Fatalf("encrypt initial file: %v", err)
	}
	if err := os.WriteFile(plainPath, []byte("TOKEN=new\n"), 0o600); err != nil {
		t.Fatalf("write updated plaintext: %v", err)
	}

	if _, err := updateOneFile(plainPath, []byte("1234")); err != nil {
		t.Fatalf("update with correct password: %v", err)
	}

	plaintext, err := cryptopkg.Decrypt(encPath, []byte("1234"))
	if err != nil {
		t.Fatalf("decrypt updated encrypted file: %v", err)
	}
	defer zeroBytes(plaintext)
	if string(plaintext) != "TOKEN=new\n" {
		t.Fatalf("updated encrypted file got=%q want=%q", plaintext, "TOKEN=new\n")
	}
}
