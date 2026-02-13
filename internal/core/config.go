package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultTTL is the default plaintext lifetime after unlock.
	DefaultTTL = time.Hour
	// WarningWindow is when warning notifications should fire.
	WarningWindow = 5 * time.Minute
)

// Config contains all filesystem paths used by Dotward.
type Config struct {
	AppDir       string
	StatePath    string
	SockPath     string
	SettingsPath string
	DefaultTTL   time.Duration
}

// ResolveConfig resolves application paths for the current user.
func ResolveConfig() (Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to resolve user config dir: %w", err)
	}

	appDir := filepath.Join(configDir, "Dotward")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to resolve home dir: %w", err)
	}

	settingsPath := filepath.Join(appDir, "config.json")
	defaultTTL, err := loadDefaultTTL(settingsPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to load config file %q: %w", settingsPath, err)
	}

	return Config{
		AppDir:       appDir,
		StatePath:    filepath.Join(appDir, "state.json"),
		SockPath:     filepath.Join(homeDir, ".dotward.sock"),
		SettingsPath: settingsPath,
		DefaultTTL:   defaultTTL,
	}, nil
}

// EnsureDirs creates directories needed by Dotward.
func EnsureDirs(cfg Config) error {
	if err := os.MkdirAll(cfg.AppDir, 0o700); err != nil {
		return fmt.Errorf("failed to create app dir %q: %w", cfg.AppDir, err)
	}
	if err := ensureDefaultConfigFile(cfg.SettingsPath); err != nil {
		return fmt.Errorf("failed to ensure config file %q: %w", cfg.SettingsPath, err)
	}
	return nil
}

type fileConfig struct {
	DefaultTTL string `json:"default_ttl"`
}

func defaultFileConfig() fileConfig {
	return fileConfig{
		DefaultTTL: DefaultTTL.String(),
	}
}

func ensureDefaultConfigFile(path string) error {
	if path == "" {
		return errors.New("settings path is empty")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	b, err := json.MarshalIndent(defaultFileConfig(), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode default config: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync default config: %w", err)
	}
	return nil
}

func loadDefaultTTL(path string) (time.Duration, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultTTL, nil
		}
		return 0, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg fileConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return 0, fmt.Errorf("failed to decode config json: %w", err)
	}
	if cfg.DefaultTTL == "" {
		return DefaultTTL, nil
	}

	ttl, err := time.ParseDuration(cfg.DefaultTTL)
	if err != nil {
		return 0, fmt.Errorf("invalid default_ttl %q: %w", cfg.DefaultTTL, err)
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("invalid default_ttl %q: must be > 0", cfg.DefaultTTL)
	}
	return ttl, nil
}
