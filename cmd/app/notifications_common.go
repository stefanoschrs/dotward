package main

import "time"

// Notifier sends warning and lifecycle notifications.
type Notifier interface {
	Init(extendCh chan<- string) error
	Warn(path string, expiresAt time.Time) error
	FileUnlocked(path string, ttl time.Duration) error
	FileDeleted(path string) error
	Shutdown() error
}
