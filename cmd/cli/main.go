package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/stefanos/dotward/internal/core"
	cryptopkg "github.com/stefanos/dotward/internal/crypto"
	"github.com/stefanos/dotward/internal/ipc"
	"github.com/stefanos/dotward/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		fmt.Println(version.Detailed("dotward-cli"))
		return
	}
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	path := os.Args[2]

	var err error
	switch cmd {
	case "unlock":
		err = unlock(path)
	case "update":
		err = update(path)
	case "lock":
		err = lock(path)
	case "batch-lock":
		err = batchLock(path)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dotward <unlock|update|lock> <file>")
	fmt.Fprintln(os.Stderr, "       dotward batch-lock <paths-file>")
	fmt.Fprintln(os.Stderr, "       dotward version")
}

func unlock(file string) error {
	cfg, err := core.ResolveConfig()
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}
	if err := ensureDaemonRunning(cfg.SockPath); err != nil {
		return errors.New("please start Dotward.app")
	}

	absPath, err := filepath.Abs(file)
	if err != nil {
		return fmt.Errorf("failed to resolve file path %q: %w", file, err)
	}
	encPath := absPath + ".enc"

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	if err := cryptopkg.DecryptFile(encPath, absPath, pw); err != nil {
		return fmt.Errorf("failed to decrypt %q: %w", encPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := ipc.Call(ctx, cfg.SockPath, "Manager.Register", ipc.Request{Path: absPath, TTL: cfg.DefaultTTL})
	if err != nil {
		_ = core.SecureDelete(absPath)
		return errors.New("please start Dotward.app")
	}
	if !resp.Success {
		_ = core.SecureDelete(absPath)
		return fmt.Errorf("daemon rejected register: %s", resp.Error)
	}

	fmt.Printf("Unlocked %s for %s\n", absPath, cfg.DefaultTTL)
	return nil
}

func update(file string) error {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return fmt.Errorf("failed to resolve file path %q: %w", file, err)
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plaintext file %q does not exist", absPath)
		}
		return fmt.Errorf("failed to stat plaintext file %q: %w", absPath, err)
	}

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	encPath := absPath + ".enc"
	if err := cryptopkg.EncryptFile(absPath, encPath, pw); err != nil {
		return fmt.Errorf("failed to encrypt %q: %w", absPath, err)
	}
	fmt.Printf("Updated encrypted file %s\n", encPath)
	return nil
}

func lock(file string) error {
	cfg, err := core.ResolveConfig()
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	absPath, err := filepath.Abs(file)
	if err != nil {
		return fmt.Errorf("failed to resolve file path %q: %w", file, err)
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plaintext file %q does not exist", absPath)
		}
		return fmt.Errorf("failed to stat plaintext file %q: %w", absPath, err)
	}

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	encPath, err := lockOneFile(absPath, pw)
	if err != nil {
		return err
	}

	if err := stopWatching(cfg.SockPath, absPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: locked file locally but failed to stop watching %s (%v)\n", absPath, err)
	}

	fmt.Printf("Locked %s and updated %s\n", absPath, encPath)
	return nil
}

func batchLock(pathsFile string) error {
	cfg, err := core.ResolveConfig()
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	paths, err := readPathsFile(pathsFile)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no file paths found in %q", pathsFile)
	}

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	var failed int
	for _, path := range paths {
		pwCopy := append([]byte(nil), pw...)
		encPath, lockErr := lockOneFile(path, pwCopy)
		if lockErr != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", path, lockErr)
			continue
		}

		if err := stopWatching(cfg.SockPath, path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: locked file locally but could not stop watching %s (%v)\n", path, err)
		}
		fmt.Printf("Locked %s and updated %s\n", path, encPath)
	}

	if failed > 0 {
		return fmt.Errorf("batch-lock completed with %d failure(s)", failed)
	}
	return nil
}

func lockOneFile(absPath string, pw []byte) (string, error) {
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plaintext file %q does not exist", absPath)
		}
		return "", fmt.Errorf("failed to stat plaintext file %q: %w", absPath, err)
	}

	encPath := absPath + ".enc"
	if err := cryptopkg.EncryptFile(absPath, encPath, pw); err != nil {
		return "", fmt.Errorf("failed to encrypt %q: %w", absPath, err)
	}

	if err := core.SecureDelete(absPath); err != nil {
		return "", fmt.Errorf("failed to lock file %q: %w", absPath, err)
	}
	return encPath, nil
}

func stopWatching(sockPath, absPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := ipc.Call(ctx, sockPath, "Manager.StopWatching", ipc.Request{Path: absPath})
	if err != nil {
		return fmt.Errorf("failed to contact daemon: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("daemon rejected stop watching: %s", resp.Error)
	}
	return nil
}

func readPassword(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err == nil {
		if len(strings.TrimSpace(string(pw))) == 0 {
			return nil, errors.New("password cannot be empty")
		}
		return pw, nil
	}

	reader := bufio.NewReader(os.Stdin)
	line, readErr := reader.ReadString('\n')
	if readErr != nil {
		return nil, fmt.Errorf("failed to read password: %w", readErr)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, errors.New("password cannot be empty")
	}
	return []byte(line), nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func ensureDaemonRunning(sockPath string) error {
	conn, err := net.DialTimeout("unix", sockPath, 750*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon socket %q: %w", sockPath, err)
	}
	_ = conn.Close()
	return nil
}

func readPathsFile(listPath string) ([]string, error) {
	absListPath, err := filepath.Abs(listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve paths file %q: %w", listPath, err)
	}
	f, err := os.Open(absListPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open paths file %q: %w", absListPath, err)
	}
	defer f.Close()

	paths := make([]string, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		absPath, err := filepath.Abs(line)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %q from %q: %w", line, absListPath, err)
		}
		paths = append(paths, absPath)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed reading paths file %q: %w", absListPath, err)
	}
	return paths, nil
}
