# JEND

<p align="center">
  <img src="FullLogoLight.png" alt="JEND Logo" width="300">
</p>

JEND is a high-performance, peer-to-peer file transfer tool written in Go. It allows you to move files securely between computers—regardless of whether they are on the same WiFi or behind strict corporate firewalls—without configuring servers or opening ports.

It was built to solve the fragility of existing tools (like `scp` or `rsync`) in unstable network conditions, using modern transport protocols to guarantee delivery.

## Architecture & Internals

I built JEND to handle the edge cases that break other transfer tools. Here is how it works under the hood:

### 1. Transport Layer: QUIC (UDP)

Instead of standard TCP, JEND runs over **QUIC** (the protocol powering HTTP/3).

* **Why?** TCP suffers from head-of-line blocking; if one packet is lost, the entire connection halts. QUIC multiplexes streams, so if a packet drops on one stream, the others keep moving. This effectively saturates available bandwidth on lossy networks (like public WiFi).

### 2. Security: End-to-End Encrypted & Zero-Trust

JEND assumes the network is compromised. Every decision was made to ensure data integrity and privacy even if an attacker controls the Wi-Fi or the relay server.

* **[Password-Authenticated Key Exchange (PAKE)](https://en.wikipedia.org/wiki/Password-authenticated_key_agreement)**:
  * **The Problem**: Sending a password/code to a server allows the server to see it (Man-in-the-Middle).
  * **The Solution**: JEND uses an Augmented PAKE protocol. The sender and receiver mathematically prove they know the same 3-word code (e.g. `fast-happy-sloth`) **without ever exchanging the code itself**. This allows for a zero-knowledge handshake.
  * **Hardening**: Because short codes are prone to brute-force, I implemented **[Argon2id](https://en.wikipedia.org/wiki/Argon2)** for key derivation (Memory=64MB, Time=3). This forces an attacker to spend prohibitive CPU resources to guess a single code.

* **[Authenticated Encryption (AEAD)](https://en.wikipedia.org/wiki/Authenticated_encryption)**:
  * Once the PAKE handshake completes, the session key is not just verified—it is used to bootstrap a secure tunnel.
  * All file data is encrypted using **AES-256-GCM** (Galois/Counter Mode). This guarantees both **Confidentiality** (no one can read it) and **Integrity** (no one can tamper with it).
  * *Why this matters*: Even if you use a malicious public relay, the relay owner sees only opaque noise. They cannot see your files.

* **Resilience & Abuse Prevention**:
  * **Rate Limiting**: The public registry prevents namespace scanning by strictly throttling lookup attempts (10 RPS/5 Burst), making online brute-force attacks mathematically infeasible.
  * **No Central Data Store**: JEND is transient. Files move directly from Peer A to Peer B. No user data is ever stored on a central server.

### 3. Network Traversal: ICE & Custom Relays

Direct P2P connectivity is blocked by most NATs, and strict enterprise or school networks add further obstacles—AP isolation, mDNS filtering, and aggressive UDP blocking.

* **Discovery**: JEND first attempts local mDNS discovery (IPv4/IPv6). On networks where multicast is blocked, it falls back to cloud registry matching: two devices with the same public IP are recognized as likely co-located and given each other's local address.
* **Hole Punching**: If local discovery fails, it uses **ICE** (Interactive Connectivity Establishment) to punch holes through NATs.
* **Configurable Relays**: For strict networks, I implemented a **"Bring Your Own Relay"** system. You can point JEND at any standard TURN server (e.g., a free Oracle Cloud instance) to route traffic when P2P is impossible. Secure, private routing without vendor lock-in.
* **QR Mode — Local**: As a last resort on networks that block everything, `--qr` mode bypasses the QUIC transport entirely and serves the file over a standard HTTP server on port 8888. The receiver opens the URL in any browser—no JEND installation required. Because it runs over plain TCP, it works on networks that block all UDP traffic.
* **QR Mode — Cloud (WebRTC)**: For situations where even the local HTTP server is unreachable (different networks, cellular), `--qr --qr-mode cloud` uses **WebRTC DataChannels** for a direct, end-to-end encrypted transfer between your terminal and any browser in the world. Signaling is handled via AWS IoT Core; the actual file data travels peer-to-peer and never touches a server.

### 4. Reliability: State-Machine Resumption

Transfers over 100MB fail often.

* **Mechanism**: JEND maintains a persistent state journal on disk (`.parallel.meta`).
* **Behavior**: If the process crashes or WiFi dies, re-running the command reads the journal, verifies the file hash of downloaded chunks, and resumes exactly where it left off. No "starting over from 0%".

---

## Installation

### Windows (Winget) — Recommended

```powershell
winget install jend
```

### macOS / Linux (Homebrew)

```bash
brew install darkprince558/tap/jend
```

### Arch Linux (AUR)

```bash
yay -S jend-bin
```

### Debian / Ubuntu / Fedora / Alpine

Download the `.deb`, `.rpm`, or `.apk` files directly from the [GitHub Releases](https://github.com/darkprince558/JEND/releases) page.

### Windows (Scoop)

```powershell
scoop bucket add darkprince558 https://github.com/darkprince558/homebrew-tap
scoop install jend
```

### From Source (Developers)

If you have Go installed:

```bash
go install github.com/darkprince558/jend/cmd/jend@latest
```

## Quick Start

**Sender**:

```bash
jend send my_project.zip
# Code: happy-delta-seven
```

**Receiver**:

```bash
jend receive happy-delta-seven
```

## Power User Features

### Multi-File Sending

Pass multiple files or directories directly and JEND bundles them into a single zip archive for the transfer. The temp archive is cleaned up automatically.

```bash
jend send report.pdf data.csv screenshots/
```

This also works with `--qr`:

```bash
jend send --qr file1.txt file2.txt image.png
```

### Piping from STDIN

JEND detects when its input is a pipe and automatically treats the incoming bytes as a text transfer. No `--text` flag needed.

```bash
cat notes.txt | jend send
echo "db_password=hunter2" | jend send
git log --oneline -10 | jend send
```

This is the fastest way to share command output or filter results with someone on the same network.

### QR Code Mode

The easiest way to share a file with someone who doesn't have JEND installed, or on restrictive networks like enterprise or school Wi-Fi.

#### Local Mode (default)

Runs a local HTTP server and displays a QR code in the terminal. The receiver scans it and the file downloads directly in their browser.

```bash
jend send --qr report.pdf
```

The browser download page shows the file name, size, type, and SHA-256 hash before the download starts. Supports multiple concurrent downloads.

#### Cloud Mode (WebRTC)

Works across any network—even when the sender and receiver are on different WiFi networks or different continents. Instead of a local server, JEND establishes a **WebRTC DataChannel** directly into the receiver's browser. The transfer is end-to-end encrypted and the file data never touches a server.

```bash
jend send --qr --qr-mode cloud report.pdf
```

When Cloud Mode is active, JEND prints both a QR code and a **6-character transfer code** (e.g. `Af38HJ`) below it. The receiver can either:

* Scan the QR code on their phone, **or**
* Open `d36yyit6n9gsha.cloudfront.net/qr` in any browser and type the code manually

Multiple people can connect and download simultaneously.

#### QR Options

Both modes support download limits and time-based expiration:

```bash
# Allow only 3 downloads, expire after 30 minutes
jend send --qr --qr-limit 3 --qr-expire 30m report.pdf

# Interactive prompt to configure options
jend send --qr report.pdf
```

When `--qr` is used without explicit flags, an interactive prompt appears to let you pick mode, download limit, and expiration.

| Flag | Description | Default |
| :--- | :--- | :--- |
| `--qr-mode` | `local` or `cloud` | interactive |
| `--qr-limit N` | Max number of downloads allowed (0 = unlimited) | `0` |
| `--qr-expire 15m` | Auto-expire the QR after a duration | `0` (never) |

### Persistent Configuration

Don't want to type flags every time? Save your preferences.

```bash
# Point JEND to your private relay
jend config set-relay --url "turn:my-server.com:3478" --user "me" --pass "123"

# Now all transfers use your infrastructure securely
jend send data.ISO
```

### Automation / CI

JEND is designed to be scriptable.

```bash
# Headless mode (no UI), JSON logs, 5m timeout
jend send --headless --no-history --timeout 5m build_artifacts.tar.gz
```

### Performance Tuning

For 10Gbps+ links, you can manually tune the concurrency:

```bash
jend receive --concurrency 16
```

## Command Reference

### `jend send`

Usage: `jend send [file] [flags]`

| Feature | Flag | Description |
| :--- | :--- | :--- |
| **Send Text** | `--text "msg"` | Send a text string directly without creating a file. Useful for sharing URLs or passwords. |
| **Multiple Files** | `file1 file2 ...` | Pass multiple files or directories as arguments. JEND bundles them into a zip automatically. |
| **Pipe from STDIN** | `cat file \| jend send` | Pipe any data into JEND and it will be sent as a text transfer. No flags needed. |
| **QR Mode** | `--qr` | Display a QR code to share a file via browser. See [QR Code Mode](#qr-code-mode) for details. |
| **QR Mode** | `--qr-mode local\|cloud` | Choose between the local HTTP server or the WebRTC cloud mode. |
| **QR Limit** | `--qr-limit N` | Cap the number of downloads allowed before the server shuts down. |
| **QR Expiry** | `--qr-expire 15m` | Auto-shut down the QR server after a duration. |
| **Incognito** | `--incognito` | Disables history logging and clipboard copying. Use this for sensitive data you don't want tracked locally. |
| **Compression** | `--tar` / `--zip` | Manually force a compression format. JEND usually detects this automatically for directories. |
| **Automation** | `--headless` | Runs without the interactive UI (TUI). Outputs machine-readable logs to stdout for scripts. |
| **S3 Mode** | `--s3` | Upload files to temporary cloud storage (AWS S3) for the receiver to download later. Useful for asynchronous transfers. |
| **Custom Relay** | `--relay-url` | Override the default relay with your own TURN server address. |

**Examples:**

```bash
# Bundle multiple files into one transfer
jend send notes.txt schema.sql diagram.png

# Pipe grep output directly to a colleague
grep -r "BUG" ./src | jend send

# Share with someone on the same WiFi (browser, no JEND needed)
jend send --qr presentation.pptx

# Share with someone anywhere in the world (different network, no JEND needed)
jend send --qr --qr-mode cloud presentation.pptx

# Limit to 1 download, expire after 15 minutes
jend send --qr --qr-limit 1 --qr-expire 15m secret.pdf

# Send a sensitive string without logging it
jend send --incognito --text "MySecretPassword"

# Run in a script (CI/CD)
jend send --headless --zip ./dist/
```

### `jend receive`

Usage: `jend receive [code] [flags]`

| Feature | Flag | Description |
| :--- | :--- | :--- |
| **Concurrency** | `--concurrency <N>` | Number of parallel QUIC streams to open (default: 4). Increase this on high-speed networks (1Gbps+). |
| **Output Path** | `--output <dir>` | Specify where to save the incoming file. Defaults to the current directory. |
| **Automation** | `--headless` | Runs without the UI. Useful for background jobs. |

**Examples:**

```bash
# Download to a specific folder with high concurrency
jend receive --output ~/Downloads --concurrency 16 happy-delta-seven
```

### `jend config`

Persistent configuration to save your preferences globally.

* `jend config set-relay` — Save your private TURN server credentials.
* `jend config clear-relay` — Reset to default relay settings.
* `jend config set-theme <name>` — Manually set the color theme (auto, dark, light, dracula, nord, catppuccin, solarized).
* `jend config set-color <name> <hex>` — Override specific colors in your active palette.

### `jend history`

JEND maintains an audit log of all your transfers locally. Running `jend history` launches an interactive TUI where you can:

* Scroll through past transfers and view file details, hashes, and durations.
* Filter by Sent/Received or search by filename/code.
* Sort by Date, Size, or Duration.
* Run `--headless` to print the log as a standard terminal table instead.

### `jend theme`

JEND includes a fully interactive theme picker.

Run `jend theme` to see a live-updating mockup of the JEND interface. As you scroll through the available built-in palettes (such as Dracula, Nord, or Catppuccin), the colors will update instantly. Press `enter` to save your choice. JEND also supports an `auto` theme mode which detects your terminal's background color automatically.
