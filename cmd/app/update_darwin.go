//go:build darwin

package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) applyUpdate(ctx context.Context, update updateNotification) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	targetAppPath, err := appBundlePath(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve app bundle path: %w", err)
	}

	tmpRoot, err := os.MkdirTemp("", "dotward-update-*")
	if err != nil {
		return fmt.Errorf("failed to create update temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	zipPath := filepath.Join(tmpRoot, "Dotward.app.zip")
	if err := downloadFile(ctx, update.AppDownloadURL, zipPath); err != nil {
		return err
	}

	extractPath := filepath.Join(tmpRoot, "extract")
	if err := unzipArchive(zipPath, extractPath); err != nil {
		return fmt.Errorf("failed to unzip downloaded update: %w", err)
	}

	newAppPath, err := findDotwardApp(extractPath)
	if err != nil {
		return err
	}

	if err := replaceAppBundle(targetAppPath, newAppPath); err != nil {
		return err
	}

	if err := installCLIIfPresent(ctx, tmpRoot, update.CLIDownloadURL); err != nil {
		return err
	}

	cmd := exec.Command("open", targetAppPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to relaunch updated app: %w", err)
	}
	return nil
}

func installCLIIfPresent(ctx context.Context, tmpRoot string, cliURL string) error {
	cliPath, ok := resolveInstalledCLIPath()
	if !ok {
		return nil
	}

	downloadPath := filepath.Join(tmpRoot, "dotward-cli.bin")
	if err := downloadFile(ctx, cliURL, downloadPath); err != nil {
		return err
	}

	info, err := os.Stat(downloadPath)
	if err != nil {
		return fmt.Errorf("failed to inspect downloaded cli file %q: %w", downloadPath, err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		if err := os.Chmod(downloadPath, 0o755); err != nil {
			return fmt.Errorf("failed to set executable permissions on downloaded cli %q: %w", downloadPath, err)
		}
	}

	return replaceBinaryAtomically(cliPath, downloadPath)
}

func resolveInstalledCLIPath() (string, bool) {
	if p, err := exec.LookPath("dotward"); err == nil {
		return p, true
	}
	candidates := []string{
		"/usr/local/bin/dotward",
		"/opt/homebrew/bin/dotward",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "dotward"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	return "", false
}

func replaceBinaryAtomically(targetPath string, sourcePath string) error {
	parentDir := filepath.Dir(targetPath)
	stagePath := filepath.Join(parentDir, fmt.Sprintf(".dotward.stage-%d", time.Now().UnixNano()))
	backupPath := filepath.Join(parentDir, fmt.Sprintf(".dotward.backup-%d", time.Now().UnixNano()))

	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open staged cli binary %q: %w", sourcePath, err)
	}
	stage, err := os.OpenFile(stagePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = src.Close()
		return fmt.Errorf("failed to create staged cli binary %q: %w", stagePath, err)
	}
	if _, err := io.Copy(stage, src); err != nil {
		_ = src.Close()
		_ = stage.Close()
		return fmt.Errorf("failed to stage cli binary for install: %w", err)
	}
	if err := stage.Sync(); err != nil {
		_ = src.Close()
		_ = stage.Close()
		return fmt.Errorf("failed to sync staged cli binary %q: %w", stagePath, err)
	}
	if err := stage.Close(); err != nil {
		_ = src.Close()
		return fmt.Errorf("failed to close staged cli binary %q: %w", stagePath, err)
	}
	if err := src.Close(); err != nil {
		return fmt.Errorf("failed to close staged source cli binary %q: %w", sourcePath, err)
	}

	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup existing cli binary %q: %w", targetPath, err)
	}
	if err := os.Rename(stagePath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("failed to install updated cli binary to %q: %w", targetPath, err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("installed updated cli but failed to remove backup %q: %w", backupPath, err)
	}
	return nil
}

func appBundlePath(exePath string) (string, error) {
	const marker = ".app/Contents/MacOS/"
	idx := strings.Index(exePath, marker)
	if idx == -1 {
		return "", fmt.Errorf("executable is not inside a .app bundle")
	}
	return exePath[:idx+len(".app")], nil
}

func downloadFile(ctx context.Context, url string, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create update download request: %w", err)
	}
	req.Header.Set("User-Agent", "dotward-app-updater")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download update from %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download update from %q: %s", url, resp.Status)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create update archive %q: %w", dst, err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to save update archive %q: %w", dst, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to sync update archive %q: %w", dst, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close update archive %q: %w", dst, err)
	}
	return nil
}

func unzipArchive(zipPath string, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip %q: %w", zipPath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("failed to create unzip destination %q: %w", dst, err)
	}

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || strings.Contains(cleanName, "../") {
			return fmt.Errorf("unsafe path in zip archive: %q", f.Name)
		}

		outPath := filepath.Join(dst, cleanName)
		if !strings.HasPrefix(outPath, dst+string(os.PathSeparator)) && outPath != dst {
			return fmt.Errorf("zip path escapes destination: %q", f.Name)
		}

		mode := f.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink entry in zip archive: %q", f.Name)
		}
		if mode.IsDir() {
			if err := os.MkdirAll(outPath, mode.Perm()); err != nil {
				return fmt.Errorf("failed to create unzip directory %q: %w", outPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
			return fmt.Errorf("failed to create unzip parent directory for %q: %w", outPath, err)
		}

		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to read zip entry %q: %w", f.Name, err)
		}
		perm := mode.Perm()
		if perm == 0 {
			perm = 0o600
		}
		dstFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("failed to create unzip file %q: %w", outPath, err)
		}
		if _, err := io.Copy(dstFile, src); err != nil {
			_ = src.Close()
			_ = dstFile.Close()
			return fmt.Errorf("failed to extract zip entry %q: %w", f.Name, err)
		}
		if err := dstFile.Close(); err != nil {
			_ = src.Close()
			return fmt.Errorf("failed to close unzip file %q: %w", outPath, err)
		}
		if err := src.Close(); err != nil {
			return fmt.Errorf("failed to close zip entry %q: %w", f.Name, err)
		}
	}
	return nil
}

func findDotwardApp(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "Dotward.app" {
			found = path
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to inspect extracted update: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("downloaded update does not contain Dotward.app")
	}
	return found, nil
}

func replaceAppBundle(targetPath string, sourcePath string) error {
	parentDir := filepath.Dir(targetPath)
	stagePath := filepath.Join(parentDir, fmt.Sprintf(".Dotward.app.stage-%d", time.Now().UnixNano()))
	backupPath := filepath.Join(parentDir, fmt.Sprintf(".Dotward.app.backup-%d", time.Now().UnixNano()))

	if err := os.RemoveAll(stagePath); err != nil {
		return fmt.Errorf("failed to prepare update stage path %q: %w", stagePath, err)
	}
	if err := copyDir(sourcePath, stagePath); err != nil {
		return fmt.Errorf("failed to stage downloaded app update: %w", err)
	}

	targetExists := true
	if _, err := os.Stat(targetPath); err != nil {
		if os.IsNotExist(err) {
			targetExists = false
		} else {
			return fmt.Errorf("failed to inspect installed app bundle %q: %w", targetPath, err)
		}
	}

	if targetExists {
		if err := os.Rename(targetPath, backupPath); err != nil {
			return fmt.Errorf("failed to move current app bundle to backup: %w", err)
		}
	}

	if err := os.Rename(stagePath, targetPath); err != nil {
		if targetExists {
			_ = os.Rename(backupPath, targetPath)
		}
		return fmt.Errorf("failed to install downloaded app bundle: %w", err)
	}

	if targetExists {
		if err := os.RemoveAll(backupPath); err != nil {
			return fmt.Errorf("installed update but failed to remove backup %q: %w", backupPath, err)
		}
	}

	return nil
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %q: %w", path, err)
		}
		target := filepath.Join(dst, relPath)

		if info.IsDir() {
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("failed to create directory %q: %w", target, err)
			}
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("failed to read symlink %q: %w", path, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("failed to create destination parent for symlink %q: %w", target, err)
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return fmt.Errorf("failed to create symlink %q -> %q: %w", target, linkTarget, err)
			}
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("failed to create destination parent for file %q: %w", target, err)
		}

		in, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open source file %q: %w", path, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return fmt.Errorf("failed to open destination file %q: %w", target, err)
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			return fmt.Errorf("failed to copy %q to %q: %w", path, target, err)
		}
		if err := out.Close(); err != nil {
			_ = in.Close()
			return fmt.Errorf("failed to close destination file %q: %w", target, err)
		}
		if err := in.Close(); err != nil {
			return fmt.Errorf("failed to close source file %q: %w", path, err)
		}
		return nil
	})
}
