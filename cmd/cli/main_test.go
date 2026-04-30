package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stefanos/dotward/internal/core"
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
	if _, err := updateOneFile(plainPath, []byte("12345"), false); err == nil {
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

	if _, err := updateOneFile(plainPath, []byte("1234"), false); err != nil {
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

func TestUpdateOneFileRefusesWhenEncMissingButDaemonReportsWatching(t *testing.T) {
	oldCfg := resolveUpdateConfig
	oldIPC := ipcIsWatching
	t.Cleanup(func() {
		resolveUpdateConfig = oldCfg
		ipcIsWatching = oldIPC
	})

	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")

	if err := os.WriteFile(plainPath, []byte("K=v\n"), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}

	resolveUpdateConfig = func() (core.Config, error) {
		return core.Config{SockPath: "/ignored.sock"}, nil
	}
	ipcIsWatching = func(ctx context.Context, sockPath, plainPath string) (bool, error) {
		return true, nil
	}

	if _, err := updateOneFile(plainPath, []byte("any-password"), true); err == nil {
		t.Fatal("expected error when .enc is missing but plaintext path is watched")
	}
}

func TestUpdateOneFileAllowsEncryptWhenEncMissingAndNotWatched(t *testing.T) {
	oldCfg := resolveUpdateConfig
	oldIPC := ipcIsWatching
	t.Cleanup(func() {
		resolveUpdateConfig = oldCfg
		ipcIsWatching = oldIPC
	})

	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")
	encPath := plainPath + ".enc"

	if err := os.WriteFile(plainPath, []byte("K=v\n"), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}

	resolveUpdateConfig = func() (core.Config, error) {
		return core.Config{SockPath: "/ignored.sock"}, nil
	}
	ipcIsWatching = func(ctx context.Context, sockPath, plainPath string) (bool, error) {
		return false, nil
	}

	if _, err := updateOneFile(plainPath, []byte("freshpw"), true); err != nil {
		t.Fatalf("update: %v", err)
	}
	plaintext, err := cryptopkg.Decrypt(encPath, []byte("freshpw"))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	defer zeroBytes(plaintext)
	if string(plaintext) != "K=v\n" {
		t.Fatalf("got %q want %q", plaintext, "K=v\n")
	}
}

func TestUpdateOneFileAllowsFirstEncryptWhenDaemonUnreachable(t *testing.T) {
	old := resolveUpdateConfig
	defer func() { resolveUpdateConfig = old }()

	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")
	encPath := plainPath + ".enc"
	sock := filepath.Join(dir, "no-such-listener.sock")

	if err := os.WriteFile(plainPath, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}

	resolveUpdateConfig = func() (core.Config, error) {
		return core.Config{SockPath: sock}, nil
	}

	if _, err := updateOneFile(plainPath, []byte("solo"), true); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := os.Stat(encPath); err != nil {
		t.Fatalf("expected encrypted file: %v", err)
	}
}

func TestUpdateOneFileRejectsWhenEncMissingWithoutCreateFlag(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, ".env")
	encPath := plainPath + ".enc"

	if err := os.WriteFile(plainPath, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if _, err := os.Stat(encPath); !os.IsNotExist(err) {
		t.Fatalf("expected no encrypted file yet, stat err=%v", err)
	}

	if _, err := updateOneFile(plainPath, []byte("any-password"), false); err == nil {
		t.Fatal("expected error when .enc is missing and --create was not requested")
	}
	if _, err := os.Stat(encPath); err == nil {
		t.Fatal("did not expect encrypted file to be created without --create")
	}
}
