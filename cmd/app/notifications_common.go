package main

import "time"

type updateNotification struct {
	Version        string
	PublishedAt    time.Time
	AppDownloadURL string
	CLIDownloadURL string
}

// Notifier sends warning and lifecycle notifications.
type Notifier interface {
	Init(extendCh chan<- string, updateCh chan<- updateNotification, skipVersionCh chan<- string) error
	Warn(path string, expiresAt time.Time) error
	FileUnlocked(path string, ttl time.Duration) error
	FileDeleted(path string) error
	UpdateAvailable(update updateNotification) error
	Shutdown() error
}
