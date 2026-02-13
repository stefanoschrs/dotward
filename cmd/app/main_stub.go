//go:build !darwin

package main

import "log"

func main() {
	log.Fatal("Dotward.app is supported on macOS only")
}
