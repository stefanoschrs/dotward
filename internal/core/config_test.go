package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultTTLMissingFileUsesDefault(t *testing.T) {
	got, err := loadDefaultTTL(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("load default ttl: %v", err)
	}
	if got != DefaultTTL {
		t.Fatalf("ttl mismatch got=%s want=%s", got, DefaultTTL)
	}
}

func TestLoadDefaultTTLFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"default_ttl":"10m"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadDefaultTTL(cfgPath)
	if err != nil {
		t.Fatalf("load default ttl: %v", err)
	}
	if got != 10*time.Minute {
		t.Fatalf("ttl mismatch got=%s want=%s", got, 10*time.Minute)
	}
}

func TestLoadDefaultTTLRejectsInvalidValues(t *testing.T) {
	tests := []string{
		`{"default_ttl":"abc"}`,
		`{"default_ttl":"0s"}`,
		`{"default_ttl":"-5m"}`,
	}

	for _, raw := range tests {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(cfgPath, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if _, err := loadDefaultTTL(cfgPath); err == nil {
			t.Fatalf("expected error for config %s", raw)
		}
	}
}

func TestEnsureDirsCreatesDefaultConfigFile(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "Dotward")
	cfg := Config{
		AppDir:       appDir,
		SettingsPath: filepath.Join(appDir, "config.json"),
	}

	if err := EnsureDirs(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	b, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var fc fileConfig
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if fc.DefaultTTL != DefaultTTL.String() {
		t.Fatalf("default_ttl mismatch got=%q want=%q", fc.DefaultTTL, DefaultTTL.String())
	}
}

func TestEnsureDirsDoesNotOverwriteExistingConfig(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "Dotward")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(appDir, "config.json")
	existing := []byte("{\"default_ttl\":\"10m\"}\n")
	if err := os.WriteFile(settingsPath, existing, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	cfg := Config{
		AppDir:       appDir,
		SettingsPath: settingsPath,
	}
	if err := EnsureDirs(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(got) != string(existing) {
		t.Fatalf("settings file was overwritten")
	}
}
