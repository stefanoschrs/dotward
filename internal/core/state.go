package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WatchedFile describes a plaintext file managed by the daemon.
type WatchedFile struct {
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expires_at"`
	Warned    bool      `json:"warned"`
}

// State holds all watched files and persists them to disk.
type State struct {
	mu    sync.Mutex
	files map[string]WatchedFile
}

// NewState creates an empty state.
func NewState() *State {
	return &State{files: make(map[string]WatchedFile)}
}

// LoadState loads state from disk. Missing state file is not an error.
func LoadState(path string) (*State, error) {
	s := NewState()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("failed to read state file %q: %w", path, err)
	}

	var list []WatchedFile
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("failed to decode state file %q: %w", path, err)
	}
	for _, wf := range list {
		s.files[wf.Path] = wf
	}
	return s, nil
}

// Save writes the state atomically as JSON with strict file permissions.
func (s *State) Save(path string) error {
	s.mu.Lock()
	list := make([]WatchedFile, 0, len(s.files))
	for _, wf := range s.files {
		list = append(list, wf)
	}
	s.mu.Unlock()

	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open temp state file %q: %w", tmpPath, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write temp state file %q: %w", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to sync temp state file %q: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp state file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace state file %q: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("failed to set permissions on state file %q: %w", path, err)
	}

	dir := filepath.Dir(path)
	df, err := os.Open(dir)
	if err == nil {
		_ = df.Sync()
		_ = df.Close()
	}

	return nil
}

// Register adds or updates a watched file.
func (s *State) Register(path string, expiresAt time.Time) {
	s.mu.Lock()
	s.files[path] = WatchedFile{Path: path, ExpiresAt: expiresAt, Warned: false}
	s.mu.Unlock()
}

// StopWatching removes a file from state.
func (s *State) StopWatching(path string) {
	s.mu.Lock()
	delete(s.files, path)
	s.mu.Unlock()
}

// Extend extends the TTL for a watched file.
func (s *State) Extend(path string, delta time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wf, ok := s.files[path]
	if !ok {
		return false
	}
	wf.ExpiresAt = wf.ExpiresAt.Add(delta)
	wf.Warned = false
	s.files[path] = wf
	return true
}

// Snapshot returns a copy of the current state map.
func (s *State) Snapshot() map[string]WatchedFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]WatchedFile, len(s.files))
	for k, v := range s.files {
		out[k] = v
	}
	return out
}

// MarkWarned marks a file as warned.
func (s *State) MarkWarned(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wf, ok := s.files[path]
	if !ok {
		return false
	}
	wf.Warned = true
	s.files[path] = wf
	return true
}

// Count returns the number of watched files.
func (s *State) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.files)
}
