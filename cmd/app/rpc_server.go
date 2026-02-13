package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"time"

	"github.com/stefanos/dotward/internal/core"
	"github.com/stefanos/dotward/internal/ipc"
)

// Manager exposes daemon RPC methods.
type Manager struct {
	state    *core.State
	cfg      core.Config
	notifier Notifier
}

// Register starts watching a plaintext file.
func (m *Manager) Register(req ipc.Request, resp *ipc.Response) error {
	if req.Path == "" {
		resp.Success = false
		resp.Error = "path is required"
		return nil
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.cfg.DefaultTTL
	}

	m.state.Register(req.Path, time.Now().Add(ttl))
	if err := m.state.Save(m.cfg.StatePath); err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("failed to save state: %v", err)
		return nil
	}
	if m.notifier != nil {
		if err := m.notifier.FileUnlocked(req.Path, ttl); err != nil {
			log.Printf("failed to send unlocked notification for %q: %v", req.Path, err)
		}
	}
	resp.Success = true
	return nil
}

// Extend extends a watched file by TTL (or default TTL if omitted).
func (m *Manager) Extend(req ipc.Request, resp *ipc.Response) error {
	if req.Path == "" {
		resp.Success = false
		resp.Error = "path is required"
		return nil
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.cfg.DefaultTTL
	}
	if ok := m.state.Extend(req.Path, ttl); !ok {
		resp.Success = false
		resp.Error = "file is not currently watched"
		return nil
	}
	if err := m.state.Save(m.cfg.StatePath); err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("failed to save state: %v", err)
		return nil
	}
	resp.Success = true
	return nil
}

// StopWatching removes a file from watch state.
func (m *Manager) StopWatching(req ipc.Request, resp *ipc.Response) error {
	if req.Path == "" {
		resp.Success = false
		resp.Error = "path is required"
		return nil
	}
	m.state.StopWatching(req.Path)
	if err := m.state.Save(m.cfg.StatePath); err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("failed to save state: %v", err)
		return nil
	}
	resp.Success = true
	return nil
}

func startRPCServer(cfg core.Config, state *core.State, notifier Notifier) (func() error, error) {
	if err := os.Remove(cfg.SockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to remove old socket %q: %w", cfg.SockPath, err)
	}

	dir := filepath.Dir(cfg.SockPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create socket dir %q: %w", dir, err)
	}

	manager := &Manager{state: state, cfg: cfg, notifier: notifier}
	server := rpc.NewServer()
	if err := server.RegisterName("Manager", manager); err != nil {
		return nil, fmt.Errorf("failed to register rpc manager: %w", err)
	}

	ln, err := net.Listen("unix", cfg.SockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %q: %w", cfg.SockPath, err)
	}
	if err := os.Chmod(cfg.SockPath, 0o700); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	go server.Accept(ln)
	return func() error {
		errClose := ln.Close()
		errRm := os.Remove(cfg.SockPath)
		if errClose != nil {
			return fmt.Errorf("failed to close rpc listener: %w", errClose)
		}
		if errRm != nil && !errors.Is(errRm, os.ErrNotExist) {
			return fmt.Errorf("failed to remove socket file: %w", errRm)
		}
		return nil
	}, nil
}
