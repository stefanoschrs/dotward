# Development Guide

This document covers the technical details, build process, and release workflow for Dotward.

## Project Layout

* `cmd/cli`: The terminal tool source (`dotward`).
* `cmd/app`: The menu bar daemon source (`Dotward.app`).
* `internal/crypto`: `AES-256-GCM` and `Argon2id` implementations.
* `internal/core`: Shared configuration and state logic.
* `internal/ipc`: RPC protocol for CLI-Daemon communication.
* `scripts/`: helper scripts for bundling the macOS App.

## Prerequisites

* **Go:** 1.23 or newer.
* **Xcode:** Command Line Tools are required for CGO (used for native notifications).
    ```bash
    xcode-select --install
    ```

## Build Commands

We use a `Makefile` to manage builds.

| Command | Description |
| :--- | :--- |
| `make build` | Builds both the CLI and the `.app` bundle to `dist/`. |
| `make install` | Builds and installs the binary to `$GOPATH/bin` and the App to `/Applications`. |
| `make test` | Runs unit tests. |
| `make clean` | Removes `dist/` artifacts. |

### building with Version Metadata
To inject version info into the binary (used by `dotward version`), pass flags to `make`:

```bash
make build VERSION=v0.2.0 COMMIT=$(git rev-parse --short HEAD)

```

## Runtime Architecture

Dotward consists of two parts:

1. **The Daemon (`Dotward.app`):**
* Runs in the background (UIElement).
* Listens on a Unix Domain Socket: `~/.dotward.sock`.
* Manages the "State Map" of unlocked files.
* Handles the Timer Loop and macOS Notifications.


2. **The CLI (`dotward`):**
* Handles user input/password prompts.
* Performs the actual Encryption/Decryption.
* Sends RPC calls (`Register`, `Deregister`) to the Daemon.



## Release Process

Releases are automated via GitHub Actions.

1. Commit your changes.
2. Tag the commit with a semantic version (e.g., `v1.0.0`).
```bash
git tag v1.0.0
git push origin v1.0.0

```


3. The workflow will build for `arm64` and `amd64`, sign the binaries, and create a GitHub Release.