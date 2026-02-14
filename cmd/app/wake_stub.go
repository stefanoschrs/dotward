//go:build !darwin

package main

func initWakeMonitor(_ chan<- struct{}) error {
	return nil
}
