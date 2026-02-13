# Dotward

Dotward is a macOS-first security tool for just-in-time access to local secret files (for example `.env`).

It keeps secrets encrypted at rest (`.env.enc`) and only decrypts them temporarily when needed. A background menu bar app monitors unlocked plaintext files and deletes them automatically when their TTL expires.

## Why Dotward

Storing plaintext environment files on disk increases risk if a laptop is lost, stolen, or compromised. Dotward reduces that risk by:

- Encrypting secret files with strong authenticated encryption.
- Limiting plaintext lifetime through automatic expiry.
- Supporting one-click lock flows for single and multiple files.
- Persisting watch state so stale plaintext is cleaned up on next daemon start.

## Features

- `AES-256-GCM` encryption with per-file random salt and nonce.
- `Argon2id` password-based key derivation.
- Secure file permissions (`0600`) for state and plaintext outputs.
- Menu bar daemon with expiry monitoring and native notifications.
- RPC-based CLI/daemon communication over Unix socket (`~/.dotward.sock`).
- `batch-lock` command to re-encrypt many files with one password prompt.

## Security Model

- Encrypted files (`*.enc`) are safe to keep on disk.
- Plaintext files are temporary and monitored by the daemon.
- If a monitored file expires, Dotward deletes it.
- If the daemon crashes, it restores state on next launch and purges expired files immediately.

Important:

- Dotward is a local hardening tool, not a replacement for full secret management platforms.
- If a machine is already actively compromised while a file is unlocked, plaintext can still be exposed.

## Requirements

- macOS (menu bar app and native notifications are macOS-specific).
- Go 1.23+
- Xcode Command Line Tools (`xcode-select --install`) for CGO/macOS framework linking.

## Project Layout

- `cmd/cli`: CLI (`dotward`).
- `cmd/app`: menu bar daemon (`Dotward.app`).
- `internal/crypto`: encryption and decryption logic.
- `internal/core`: state/config/file utilities.
- `internal/ipc`: RPC request/response and client.
- `scripts/build_app.sh`: `.app` bundle build script.
- `dist/`: build artifacts (ignored by git).

## Build

Build both CLI and app bundle:

```bash
make build
```

Artifacts:

- `dist/dotward`
- `dist/Dotward.app`

Build with explicit version metadata:

```bash
make build VERSION=v0.1.0 COMMIT=$(git rev-parse --short HEAD) BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
```

Build only CLI:

```bash
make build-cli
```

Build only app bundle:

```bash
make build-app
```

Run tests:

```bash
make test
```

Format Go files:

```bash
make fmt
```

Clean build artifacts:

```bash
make clean
```

## Install CLI

Install `dotward` into your `GOBIN` (or `$(go env GOPATH)/bin` if `GOBIN` is unset):

```bash
make install
```

After install, ensure your shell PATH includes that directory.

## Versioning

Dotward follows semantic versioning (`vMAJOR.MINOR.PATCH`) and embeds build metadata in both binaries.

Check installed CLI version metadata:

```bash
dotward version
```

This prints:

- version
- commit
- build timestamp (UTC)
- builder identifier

App versioning is embedded at build time and also written to `Info.plist`:

- `CFBundleVersion`
- `CFBundleShortVersionString`

## Install and Run App

1. Build app bundle: `make build-app`
2. Move `dist/Dotward.app` to `/Applications`.
3. Launch `Dotward.app`.
4. Grant notification permissions when prompted.

## Usage

### 1) Encrypt or update encrypted file

```bash
dotward update /absolute/or/relative/path/to/.env
```

Prompts for password and writes `/path/to/.env.enc`.

### 2) Unlock file temporarily

```bash
dotward unlock /absolute/or/relative/path/to/.env
```

- Prompts for password.
- Decrypts `/path/to/.env.enc` into `/path/to/.env`.
- Registers the plaintext path with daemon monitoring.

If daemon is not running, unlock fails closed with:

- `please start Dotward.app`

### 3) Lock file immediately

```bash
dotward lock /absolute/or/relative/path/to/.env
```

- Prompts for password.
- Re-encrypts plaintext to `.enc`.
- Securely deletes plaintext file.
- Requests daemon to stop watching that file.

### 4) Batch lock multiple files with one password prompt

Create a text file containing one file path per line:

```text
/Users/alice/work/service-a/.env
/Users/alice/work/service-b/.env.dev
# comments and blank lines are ignored
```

Run:

```bash
dotward batch-lock /path/to/paths.txt
```

Each file is processed in place:

- `/path/to/.env` -> `/path/to/.env.enc`
- plaintext file is deleted

## Runtime Behavior

- Daemon state file: `~/Library/Application Support/Dotward/state.json`
- User config file: `~/Library/Application Support/Dotward/config.json`
- RPC socket: `~/.dotward.sock`
- Default unlock TTL: `1h` (configurable via `config.json`)
- Warning window: `5m` before expiry

Example `config.json`:

```json
{
  "default_ttl": "10m"
}
```

Menu bar behavior:

- Lock icon always visible.
- Red numeric badge shown when one or more files are monitored.
- Dropdown lists monitored files; clicking one locks it immediately.

## Troubleshooting

### `please start Dotward.app`

The daemon is not running or socket is unavailable.

- Launch `/Applications/Dotward.app`.
- Retry command.

### Build/link errors mentioning Apple frameworks

Install/update Xcode CLI tools:

```bash
xcode-select --install
```

### Notifications do not appear

Check macOS notification permissions for Dotward in System Settings.

## Releases

Releases are automated via GitHub Actions on tags.

- Trigger: push a tag matching `v*` (for example `v1.2.0`)
- Workflow: `.github/workflows/release.yml`
- Published assets:
  - versioned CLI binary (macOS target)
  - zipped `Dotward.app`
  - SHA256 checksums file

Example:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Development Notes

- Keep dependencies minimal and security-sensitive code explicit.
- Prefer standard library except where macOS integration requires CGO.
- Follow `AGENTS.md` and `INSTRUCTIONS.md` for contribution standards.

## License

Add your project license file (for example `MIT`) before public release.
