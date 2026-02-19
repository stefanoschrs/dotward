# Dotward

<p align="center">
    <picture>
        <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/stefanoschrs/dotward/master/AppIcons/Assets.xcassets/AppIcon.appiconset/128.png">
        <img src="https://raw.githubusercontent.com/stefanoschrs/dotward/master/AppIcons/Assets.xcassets/AppIcon.appiconset/128.png" alt="Dotward" width="128">
    </picture>
</p>

**Dotward** is a macOS security tool for **Just-In-Time (JIT) access** to local secret files (like `.env`).

It bridges the gap between security and convenience by keeping your secrets encrypted at rest (`.env.enc`) and only decrypting them temporarily when you are actually working. A background menu bar app monitors these files and **automatically deletes them** when their time expires.



## Why Dotward?

Leaving plaintext `.env` files on your hard drive is a security risk. If your laptop is stolen or compromised, those secrets are exposed. Dotward mitigates this by:

* **Encryption at Rest:** Files are stored as `*.enc` using `AES-256-GCM`.
* **Auto-Expiry:** Plaintext files self-destruct after 1 hour (configurable).
* **Native Integration:** MacOS notifications let you extend access without context switching.

## Installation

### Option 1: Binary (Recommended)
Download the latest release for your architecture (`arm64` for M1/M2/M3, `amd64` for Intel) from the [Releases Page](https://github.com/stefanoschrs/dotward/releases).

1.  **Install the App:**
    Unzip `Dotward.app.zip` and drag `Dotward.app` to your `/Applications` folder. Launch it.
2.  **Install the CLI:**
    Move the `dotward` binary to your path:
    ```bash
    sudo mv dotward /usr/local/bin/
    sudo chmod +x /usr/local/bin/dotward
    ```

### Option 2: Build from Source
See [DEVELOPMENT.md](DEVELOPMENT.md) for build instructions.

## Usage Workflow

### 1. Protect a File
First, take an existing plaintext file and encrypt it. You will be prompted for a password.

```bash
# Encrypts .env -> .env.enc
dotward update .env

```

*Note: This command does not delete the original file immediately. You can delete the plaintext version once you confirm the `.enc` file exists.*

### 2. Unlock for Development

When you are ready to work, unlock the file.

```bash
# Decrypts .env.enc -> .env
dotward unlock .env

```

This registers the file with the **Dotward Daemon**. The clock starts ticking (default: 1 hour).

### 3. Notifications & Extending

Five minutes before your file expires, Dotward will send a native macOS notification:

> **"File Expiring: .env"**
> *Time remaining: 5m*

**Click the "Extend" button** on the notification to add another hour to the timer. You do not need to open a terminal.

### 4. Lock Manually

Finished early? You can lock the file immediately to scrub the plaintext from your disk.

```bash
dotward lock .env

```

## Configuration

You can customize the default Time-To-Live (TTL) by creating a config file at `~/Library/Application Support/Dotward/config.json`.

```json
{
  "default_ttl": "4h",
  "warning_window": "10m"
}

```

* `default_ttl`: How long a file stays unlocked (e.g., `30m`, `1h`, `8h`). Default is `1h`.
* `warning_window`: How soon before expiry to send the notification. Default is `5m`.

## Batch Operations

If you have many microservices, you can lock or unlock them all at once using a batch list.

1. Create a file `targets.txt` with absolute paths:
```text
/Users/me/project-a/.env
/Users/me/project-b/.env

```


2. Run batch lock:
```bash
dotward batch-lock targets.txt

```

3. Run batch unlock:
```bash
dotward batch-unlock targets.txt

```



## Security Model

* **Encryption:** Uses `Argon2id` for key derivation and `AES-256-GCM` for file encryption.
* **State Recovery:** If the daemon crashes or the machine reboots, Dotward cleans up any expired files immediately upon restart.
* **Permissions:** Output files are written with `0600` permissions (read/write by owner only).

## Contributing

For development instructions, build steps, and architecture details, please see [DEVELOPMENT.md](DEVELOPMENT.md).

## Troubleshooting

1. **Notifications not showing:**  
Make sure that they are enabled in the Settings > Notifications > Dotward
<img width="300" src="https://i.ibb.co/r2QsT6sf/Screenshot-2026-02-17-at-10-38-44.png">

## License

Licensed under the terms in [`LICENSE`](LICENSE).
