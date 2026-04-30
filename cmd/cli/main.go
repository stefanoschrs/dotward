package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/stefanos/dotward/internal/core"
	cryptopkg "github.com/stefanos/dotward/internal/crypto"
	"github.com/stefanos/dotward/internal/ipc"
	"github.com/stefanos/dotward/internal/version"
)

var permanentFlag bool

// resolveUpdateConfig is the config resolver used by update; tests may replace it.
var resolveUpdateConfig = core.ResolveConfig

// ipcIsWatching probes the daemon before allowing first-time encrypt (--create). Tests may replace it.
var ipcIsWatching = func(ctx context.Context, sockPath, plainPath string) (bool, error) {
	resp, err := ipc.Call(ctx, sockPath, "Manager.IsWatching", ipc.Request{Path: plainPath})
	if err != nil {
		return false, err
	}
	return resp.Success, nil
}

var rootCmd = &cobra.Command{
	Use:     "dotward",
	Short:   "JIT access to local secret files",
	Version: version.Detailed("dotward-cli"),
}

var unlockCmd = &cobra.Command{
	Use:   "unlock <file> [files...]",
	Short: "Decrypt one or more files and register them with the daemon",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return unlock(args, permanentFlag)
	},
}

var catCmd = &cobra.Command{
	Use:   "cat <file>",
	Short: "Decrypt and print a file to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return cat(args[0])
	},
}

var updateCmd = &cobra.Command{
	Use:   "update <file> [files...]",
	Short: "Re-encrypt one or more plaintext files",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		allowCreate, err := cmd.Flags().GetBool("create")
		if err != nil {
			return err
		}
		return update(args, allowCreate)
	},
}

var lockCmd = &cobra.Command{
	Use:   "lock <file> [files...]",
	Short: "Encrypt one or more files and securely delete the plaintext",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lock(args)
	},
}

var batchLockCmd = &cobra.Command{
	Use:   "batch-lock <paths-file>",
	Short: "Lock multiple files listed in a paths file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return batchLock(args[0])
	},
}

var batchUnlockCmd = &cobra.Command{
	Use:   "batch-unlock <paths-file>",
	Short: "Unlock multiple files listed in a paths file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return batchUnlock(args[0])
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.Detailed("dotward-cli"))
	},
}

func init() {
	unlockCmd.Flags().BoolVar(&permanentFlag, "permanent", false, "keep file unlocked until manually locked")
	updateCmd.Flags().Bool("create", false, "create a new .enc sidecar when none exists (first-time encrypt only)")
	rootCmd.AddCommand(unlockCmd, catCmd, updateCmd, lockCmd, batchLockCmd, batchUnlockCmd, versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func cat(file string) error {
	_, encPath, err := resolveUnlockPaths(file)
	if err != nil {
		return err
	}

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	plaintext, err := cryptopkg.Decrypt(encPath, pw)
	if err != nil {
		return fmt.Errorf("failed to decrypt %q: %w", encPath, err)
	}
	defer zeroBytes(plaintext)

	_, err = os.Stdout.Write(plaintext)
	return err
}

func unlock(files []string, permanent bool) error {
	var cfg core.Config
	if !permanent {
		var err error
		cfg, err = core.ResolveConfig()
		if err != nil {
			return fmt.Errorf("failed to resolve config: %w", err)
		}
		if err := ensureDaemonRunning(cfg.SockPath); err != nil {
			return errors.New("please start Dotward.app")
		}
	}

	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	var failed int
	for _, file := range files {
		pwCopy := append([]byte(nil), pw...)
		if unlockErr := unlockOnePath(file, pwCopy, permanent, cfg); unlockErr != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", file, unlockErr)
			continue
		}
		if permanent {
			fmt.Printf("Permanently unlocked %s\n", file)
		} else {
			fmt.Printf("Unlocked %s for %s\n", file, cfg.DefaultTTL)
		}
	}

	if failed > 0 {
		return fmt.Errorf("unlock completed with %d failure(s)", failed)
	}
	return nil
}

func unlockOnePath(file string, pw []byte, permanent bool, cfg core.Config) error {
	defer zeroBytes(pw)

	absPath, encPath, err := resolveUnlockPaths(file)
	if err != nil {
		return err
	}

	if err := cryptopkg.DecryptFile(encPath, absPath, pw); err != nil {
		return fmt.Errorf("failed to decrypt %q: %w", encPath, err)
	}

	if permanent {
		return nil
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
	return nil
}

func update(files []string, allowCreateMissingEnc bool) error {
	pw, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	var failed int
	for _, file := range files {
		pwCopy := append([]byte(nil), pw...)
		encPath, updateErr := updateOneFile(file, pwCopy, allowCreateMissingEnc)
		if updateErr != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", file, updateErr)
			continue
		}
		fmt.Printf("Updated encrypted file %s\n", encPath)
	}

	if failed > 0 {
		return fmt.Errorf("update completed with %d failure(s)", failed)
	}
	return nil
}

func updateOneFile(file string, pw []byte, allowCreateMissingEnc bool) (string, error) {
	defer zeroBytes(pw)

	absPath, encPath, err := resolveUnlockPaths(file)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plaintext file %q does not exist", absPath)
		}
		return "", fmt.Errorf("failed to stat plaintext file %q: %w", absPath, err)
	}

	if _, err := os.Stat(encPath); err != nil {
		if os.IsNotExist(err) {
			if !allowCreateMissingEnc {
				return "", fmt.Errorf("encrypted file %q does not exist; pass --create only for first-time encrypt from plaintext", encPath)
			}
			cfg, cfgErr := resolveUpdateConfig()
			if cfgErr == nil {
				if err := refuseUpdateWhenEncMissingIfWatched(cfg.SockPath, encPath, absPath); err != nil {
					return "", err
				}
			}
		} else {
			return "", fmt.Errorf("failed to stat encrypted file %q: %w", encPath, err)
		}
	} else {
		if err := validateExistingEncryptedFilePassword(encPath, pw); err != nil {
			return "", err
		}
	}
	if err := cryptopkg.EncryptFile(absPath, encPath, pw); err != nil {
		return "", fmt.Errorf("failed to encrypt %q: %w", absPath, err)
	}
	return encPath, nil
}

func refuseUpdateWhenEncMissingIfWatched(sockPath, encPath, absPlain string) error {
	if sockPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	watching, err := ipcIsWatching(ctx, sockPath, absPlain)
	if err != nil {
		// Daemon not running or unreachable: allow first-time encrypt with --create.
		return nil
	}
	if watching {
		return fmt.Errorf("cannot update: encrypted file %q is missing but %q is still registered as unlocked by Dotward; restore the sidecar or lock the file before updating", encPath, absPlain)
	}
	return nil
}

func validateExistingEncryptedFilePassword(encPath string, pw []byte) error {
	plaintext, err := cryptopkg.Decrypt(encPath, pw)
	if err != nil {
		return fmt.Errorf("failed to verify password for existing encrypted file %q: wrong password or corrupted file: %w", encPath, err)
	}
	defer zeroBytes(plaintext)
	return nil
}

func lock(files []string) error {
	cfg, err := core.ResolveConfig()
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	pw, err := readPasswordWithConfirmation("Password: ", "Confirm password: ")
	if err != nil {
		return err
	}
	defer zeroBytes(pw)

	var failed int
	for _, file := range files {
		absPath, err := filepath.Abs(file)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", file, err)
			continue
		}

		pwCopy := append([]byte(nil), pw...)
		encPath, lockErr := lockOneFile(absPath, pwCopy)
		if lockErr != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", file, lockErr)
			continue
		}

		if err := stopWatching(cfg.SockPath, absPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: locked file locally but failed to stop watching %s (%v)\n", absPath, err)
		}
		fmt.Printf("Locked %s and updated %s\n", absPath, encPath)
	}

	if failed > 0 {
		return fmt.Errorf("lock completed with %d failure(s)", failed)
	}
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

	pw, err := readPasswordWithConfirmation("Password: ", "Confirm password: ")
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

func batchUnlock(pathsFile string) error {
	cfg, err := core.ResolveConfig()
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}
	if err := ensureDaemonRunning(cfg.SockPath); err != nil {
		return errors.New("please start Dotward.app")
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
		unlockErr := unlockOneFile(path, pwCopy, cfg.SockPath, cfg.DefaultTTL)
		zeroBytes(pwCopy)
		if unlockErr != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAILED %s: %v\n", path, unlockErr)
			continue
		}
		fmt.Printf("Unlocked %s for %s\n", path, cfg.DefaultTTL)
	}

	if failed > 0 {
		return fmt.Errorf("batch-unlock completed with %d failure(s)", failed)
	}
	return nil
}

func unlockOneFile(path string, pw []byte, sockPath string, ttl time.Duration) error {
	absPath, encPath, err := resolveUnlockPaths(path)
	if err != nil {
		return err
	}
	if err := cryptopkg.DecryptFile(encPath, absPath, pw); err != nil {
		return fmt.Errorf("failed to decrypt %q: %w", encPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := ipc.Call(ctx, sockPath, "Manager.Register", ipc.Request{Path: absPath, TTL: ttl})
	if err != nil {
		_ = core.SecureDelete(absPath)
		return errors.New("please start Dotward.app")
	}
	if !resp.Success {
		_ = core.SecureDelete(absPath)
		return fmt.Errorf("daemon rejected register: %s", resp.Error)
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
		if len(bytes.TrimSpace(pw)) == 0 {
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

func readPasswordWithConfirmation(prompt, confirmPrompt string) ([]byte, error) {
	pw, err := readPassword(prompt)
	if err != nil {
		return nil, err
	}

	confirm, err := readPassword(confirmPrompt)
	if err != nil {
		zeroBytes(pw)
		return nil, err
	}
	defer zeroBytes(confirm)

	if len(pw) != len(confirm) || subtle.ConstantTimeCompare(pw, confirm) != 1 {
		zeroBytes(pw)
		return nil, errors.New("passwords do not match")
	}
	return pw, nil
}

func resolveUnlockPaths(path string) (string, string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve file path %q: %w", path, err)
	}

	if strings.HasSuffix(absPath, ".enc") {
		plainPath := strings.TrimSuffix(absPath, ".enc")
		if plainPath == "" {
			return "", "", fmt.Errorf("invalid encrypted file path %q", absPath)
		}
		return plainPath, absPath, nil
	}
	return absPath, absPath + ".enc", nil
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
