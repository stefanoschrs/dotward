package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Preferences stores persisted updater choices.
type Preferences struct {
	SkippedVersion string `json:"skipped_version,omitempty"`
}

// PreferenceStore handles loading and saving updater preferences.
type PreferenceStore struct {
	mu   sync.Mutex
	path string
	pref Preferences
}

// LoadPreferenceStore reads updater preferences from disk.
func LoadPreferenceStore(path string) (*PreferenceStore, error) {
	p := &PreferenceStore{path: path}

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
		return nil, fmt.Errorf("failed to read update preferences %q: %w", path, err)
	}

	if err := json.Unmarshal(b, &p.pref); err != nil {
		return nil, fmt.Errorf("failed to decode update preferences %q: %w", path, err)
	}
	return p, nil
}

// SkippedVersion returns the currently skipped release tag.
func (p *PreferenceStore) SkippedVersion() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pref.SkippedVersion
}

// SetSkippedVersion updates and persists the skipped release tag.
func (p *PreferenceStore) SetSkippedVersion(tag string) error {
	p.mu.Lock()
	p.pref.SkippedVersion = tag
	p.mu.Unlock()
	return p.save()
}

func (p *PreferenceStore) save() error {
	p.mu.Lock()
	out := p.pref
	p.mu.Unlock()

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode update preferences: %w", err)
	}
	b = append(b, '\n')

	tmpPath := p.path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open temp update preferences %q: %w", tmpPath, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write temp update preferences %q: %w", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to sync temp update preferences %q: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp update preferences %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, p.path); err != nil {
		return fmt.Errorf("failed to replace update preferences %q: %w", p.path, err)
	}
	if err := os.Chmod(p.path, 0o600); err != nil {
		return fmt.Errorf("failed to set permissions on update preferences %q: %w", p.path, err)
	}

	dir := filepath.Dir(p.path)
	df, err := os.Open(dir)
	if err == nil {
		_ = df.Sync()
		_ = df.Close()
	}
	return nil
}
