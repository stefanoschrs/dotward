# INSTRUCTIONS.md - Project: Dotward

## 1. Project Overview

**Dotward** is a macOS-native security tool written in **Golang**. It provides "Just-In-Time" access to local secret files (e.g., `.env`) for development.

**Core Problem:** Developers keep plaintext `.env` files on disk, creating a security risk if the drive is compromised.
**Solution:**

1. Files are stored encrypted on disk (e.g., `.env.enc`).
2. A CLI tool (`dotward unlock`) decrypts them temporarily.
3. A background **macOS Menu Bar App** watches these files.
4. After **1 hour**, the App automatically deletes the plaintext file.
5. **5 minutes before expiration**, a native macOS notification appears with an **"Extend" button**. Clicking it adds 1 hour to the timer without opening a terminal.

## 2. Architecture

The project consists of two binaries built from the same repository:

1. **The Daemon (`Dotward.app`):**
* Runs as a macOS Application (UIElement/Menu Bar app).
* Listens on a Unix Domain Socket (`~/.dotward.sock`).
* Manages the state of unlocked files (in-memory & `state.json`).
* Handles the Timer loop and Native Notifications (CGO).


2. **The CLI (`dotward`):**
* Standard terminal binary.
* Handles Encryption/Decryption logic.
* Communicates with the Daemon via RPC (over the Unix socket) to register/deregister files.



## 3. Tech Stack

* **Language:** Golang (1.23+)
* **Crypto:** `golang.org/x/crypto/nacl/secretbox` or `filippo.io/age` (User preference: standard, robust encryption).
* **GUI/Menu Bar:** `github.com/getlantern/systray`
* **RPC:** Native `net/rpc` over Unix Socket.
* **System Integration:** CGO (Objective-C) for `UserNotifications` framework.

## 4. Directory Structure

```text
/
├── cmd/
│   ├── cli/
│   │   └── main.go           # The 'dotward' terminal tool
│   └── app/
│       ├── main.go           # The MacOS App entry point (systray)
│       ├── notifications.go  # CGO Bridge for Notifications
│       └── rpc_server.go     # Unix Socket Listener
├── internal/
│   ├── core/                 # Shared logic
│   │   ├── config.go         # Paths & Constants
│   │   └── state.go          # State management (Load/Save JSON)
│   ├── crypto/               # Encryption/Decryption wrappers
│   └── ipc/                  # RPC definitions (Args/Response structs)
├── scripts/
│   └── build_app.sh          # Script to bundle the .app
├── go.mod
└── INSTRUCTIONS.md

```

## 5. Implementation Steps

### Phase 1: The Cryptography Engine (`internal/crypto`)

* Implement `EncryptFile(src, dst string, password string) error`.
* Implement `DecryptFile(src, dst string, password string) error`.
* **Requirement:** Use a secure KDF (like Argon2 or Scrypt) to derive the key from the password. Do not store the password.
* **Extension:** Support `.env.enc` as the standard extension.

### Phase 2: The Daemon & State (`cmd/app`)

* **State Management (`internal/core/state.go`):**
* Struct `WatchedFile`: `{ Path: string, ExpiresAt: time.Time, Warned: bool }`.
* Persist state to `~/Library/Application Support/Dotward/state.json`.
* **Crash Recovery:** On startup, load `state.json`. If any file has `ExpiresAt < Now`, delete it immediately.


* **Menu Bar (`cmd/app/main.go`):**
* Use `systray` to show a lock icon.
* Menu items:
* "Status: monitoring X files" (disabled item).
* "Quit" (cleans up socket and exits).




* **The Watcher Loop:**
* Run a `time.Ticker` (e.g., every 10s).
* Check all files in the map.
* **Logic:**
* If `Now > ExpiresAt`: `os.Remove(file)`, remove from map, notify user "File Deleted".
* If `Now > (ExpiresAt - 5m)` AND `!Warned`: Trigger Native Notification.
* If file is missing (user deleted it): Remove from map silently.





### Phase 3: Native Notifications with Actions (CGO)

* **File:** `cmd/app/notifications.go`
* **Requirement:** Must use CGO to interface with `UserNotifications` framework.
* **Delegate:** Implement a `UNUserNotificationCenterDelegate` in Objective-C within the Go file (using the preamble comments).
* **Action:**
* Register a category `EXPIRY_WARNING` with one action: `EXTEND_ACTION` ("Extend 1 Hour").
* When the user clicks the button, the Objective-C delegate calls a Go exported function `export HandleExtendAction(id *C.char)`.
* This Go function sends the File ID to the main loop channel to update `ExpiresAt += 1h` and reset `Warned = false`.



### Phase 4: The RPC Layer (`internal/ipc` & `cmd/app/rpc_server.go`)

* Define RPC methods exposed by the Daemon:
* `Manager.Register(path string) error`
* `Manager.Extend(path string) error`
* `Manager.StopWatching(path string) error`


* Daemon listens on `~/.dotward.sock`.

### Phase 5: The CLI (`cmd/cli`)

* **Commands:**
1. `dotward unlock <file>`:
* Checks if `dotward.app` is running (connect to socket). If not, error: "Please start Dotward.app".
* Prompts for password.
* Decrypts `<file>.enc` -> `<file>`.
* RPC Call: `Manager.Register(<abspath>)`.


2. `dotward update <file>`:
* Reads plaintext `<file>`.
* Prompts for password.
* Encrypts to `<file>.enc`.
* **Note:** Does NOT delete the plaintext file (that's the daemon's job).


3. `dotward lock <file>`:
* RPC Call: `Manager.StopWatching(<abspath>)`.
* Securely deletes `<file>`.





### Phase 6: Build & Packaging (`scripts/build_app.sh`)

* The Go binary `cmd/app` **cannot** run directly for Notifications to work correctly. It must be inside an `.app` bundle.
* **Script Logic:**
1. Build `cmd/app` -> `Dotward`.
2. Create directory structure `Dotward.app/Contents/MacOS`.
3. Create `Info.plist` inside `Contents`:
* `LSUIElement = true` (Hide from Dock).
* `CFBundleIdentifier = com.yourname.dotward`.


4. Copy binary to `MacOS/Dotward`.


* **Installation:** User drags `Dotward.app` to Applications.

## 6. Edge Cases & Security

1. **System Sleep:**
* If the Mac sleeps for 2 hours, the timer will have expired when it wakes.
* **Requirement:** On system wake, the ticker must immediately check and purge expired files.


2. **Force Quit:**
* If `Dotward.app` is killed, the files remain plaintext.
* **Mitigation:** On the *next* start of the App, it reads `state.json`. It will see the old timestamps and immediately delete the files.


3. **Permissions:**
* The App will need Disk Access permissions (usually prompted by macOS on first file access) and Notification permissions.



## 7. Deliverable Checklist for Agent

* [ ] `internal/crypto`: Robust Encryption/Decryption.
* [ ] `cmd/app`: CGO Notifications with `UNNotificationAction`.
* [ ] `cmd/app`: Systray implementation.
* [ ] `cmd/cli`: RPC Client implementation.
* [ ] `scripts`: `build_app.sh` to generate a valid macOS bundle.
* [ ] `README`: Usage instructions (init, unlock, update).
