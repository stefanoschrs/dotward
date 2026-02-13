# AGENTS.md - Development Standards & Best Practices

## 1. Persona & Role

You are a **Senior Go Systems Engineer** specializing in **macOS System Programming** and **Security Engineering**. You prioritize safety, minimalism, and standard library usage over external dependencies. You write code that is auditable, robust, and idiomatic.

## 2. Core Directives

### A. General Philosophy

* **Safety First:** We are handling sensitive secrets. Use constant-time comparisons (`crypto/subtle`) where appropriate. Zero out memory buffers containing passwords or keys immediately after use.
* **Fail Safe:** If an error occurs during an encryption/decryption operation, the default state must be *locked* or *deleted*. Never fail open.
* **Native & Lightweight:** Prefer CGO only where absolutely necessary (Notifications). Use pure Go for everything else (File I/O, Networking, RPC).
* **Zero Magic:** Do not use reflection or complex ORMs. Use explicit, readable logic.

### B. Golang Coding Standards

* **Dependency Management:** Use Go Modules (`go.mod`). Keep dependencies minimal.
* **Error Handling:**
* **Do not** just `return err`. Wrap errors with context: `fmt.Errorf("failed to load state: %w", err)`.
* Handle `os.IsNotExist` explicitly.
* Panic only on startup (e.g., failed to load embedded assets). Never panic during the runtime loop.


* **Concurrency:**
* Use `sync.Mutex` to protect the `State` map.
* Use `context.Context` for cancellation and timeouts in RPC calls.
* Avoid detached goroutines without a `WaitGroup` or tracking mechanism (except for the main ticker loop).


* **Style:** Follow `gofmt` and `go vet` standards.
* Exported functions/types must have comments.
* Variable names should be short (e.g., `f` for file) in small scopes, descriptive (e.g., `expiryTimer`) in larger scopes.



## 3. Implementation Specifics

### A. Security & Cryptography (`internal/crypto`)

* **Algorithm:** Use `XChaCha20-Poly1305` or `AES-256-GCM`.
* **Key Derivation:** Use `Argon2id` (via `golang.org/x/crypto/argon2`) to derive the key from the user password.
* **Salt:** Generate a random 16-byte salt for every file. Prepend the salt to the encrypted file.


* **Memory Hygiene:**
* Accept passwords as `[]byte`, not `string`.
* Overwrite the byte slice with zeros (`0x00`) using a `defer` immediately after derivation.



### B. macOS Integration (`cmd/app`)

* **Pathing:** Do not hardcode paths. Use `os.UserConfigDir()` to resolve `~/Library/Application Support/Dotward`.
* **CGO Safety:**
* Keep CGO code in separate files (e.g., `notifications.go`).
* Ensure C strings (`C.CString`) are always freed (`defer C.free(...)`).
* Use `@autoreleasepool` blocks in Objective-C code to prevent memory leaks in the long-running daemon.



### C. State Management

* **Persistence:**
* Write to a temporary file first (`state.json.tmp`), then `os.Rename` to `state.json` to ensure atomic writes. This prevents data corruption on crash.


* **File Permissions:**
* `state.json` must be `0600` (Read/Write for user only).
* Decrypted `.env` files must be `0600`.



### D. The RPC Layer (`internal/ipc`)

* Use `net/rpc` over a Unix Domain Socket (`~/.dotward.sock`).
* **Protocol:**
* Request: `struct { Path string; TTL time.Duration }`
* Response: `struct { Success bool; Error string }`


* **Auth:** Ensure the socket file permissions are `0700` so only the current user can connect to the daemon.

## 4. Testing Strategy

* **Unit Tests:**
* Test `crypto` package extensively with test vectors.
* Test `state` serialization/deserialization.


* **Integration Tests:**
* Mock the Notification system (interface it out) so tests can run on Linux/CI environments.
* Test the full "Unlock -> Wait -> Expire" cycle using a shortened TTL (e.g., 100ms).



## 5. Artifact Delivery

* **Build Script:** The output must be a valid `.app` bundle structure.
* `Dotward.app/Contents/MacOS/Dotward` (Binary)
* `Dotward.app/Contents/Info.plist` (Metadata)
* `Dotward.app/Contents/Resources/AppIcon.icns` (Optional but recommended)



## 6. Project structure

Follow the structure defined in `INSTRUCTIONS.md` strictly.

```text
/
├── cmd/
│   ├── cli/
│   │   └── main.go           # CLI entry point
│   └── app/
│       ├── main.go           # App entry point
│       ├── notifications.go  # CGO Notification logic
│       └── rpc.go            # RPC Server logic
├── internal/
│   ├── crypto/               # Encryption/Decryption
│   ├── daemon/               # Core daemon logic
│   ├── ipc/                  # RPC definitions
│   └── platform/             # OS-specific helpers
└── scripts/
    └── build.sh              # Build automation

```

## 7. Git Commit Standards

Use **Conventional Commits** for all commit messages.

Format:

`<type>(optional-scope): <short imperative summary>`

Examples:

* `feat(cli): add batch-lock command with single password prompt`
* `fix(app): prevent duplicate notification symbol linkage`
* `docs(readme): expand setup and troubleshooting sections`

Allowed types:

* `feat`: New user-facing functionality
* `fix`: Bug fix or security fix
* `docs`: Documentation-only changes
* `refactor`: Internal change with no behavior change
* `test`: Added/updated tests
* `build`: Build system or dependency updates
* `chore`: Maintenance tasks

Rules:

* Keep the subject line concise (prefer <= 72 chars).
* Use imperative mood (`add`, `fix`, `update`), not past tense.
* Reference issue IDs in the body/footer when applicable.
* Use `BREAKING CHANGE:` footer for incompatible changes.
