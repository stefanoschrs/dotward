//go:build !darwin

package main

import "time"

type noopNotifier struct{}

func newNotifier() Notifier {
	return &noopNotifier{}
}

func (n *noopNotifier) Init(_ chan<- string) error {
	return nil
}

func (n *noopNotifier) Warn(_ string, _ time.Time) error {
	return nil
}

func (n *noopNotifier) FileUnlocked(_ string, _ time.Duration) error {
	return nil
}

func (n *noopNotifier) FileDeleted(_ string) error {
	return nil
}

func (n *noopNotifier) Shutdown() error {
	return nil
}
