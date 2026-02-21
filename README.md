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
* **QR Mode (Browser Transfer)**: As a last resort on networks that block everything, `--qr` mode bypasses the QUIC transport entirely and serves the file over a standard HTTP server on port 8888. The receiver opens the URL in any browser—no JEND installation required. Because it runs over plain TCP, it works on networks that block all UDP traffic.

### 4. Reliability: State-Machine Resumption

Transfers over 100MB fail often.

* **Mechanism**: JEND maintains a persistent state journal on disk (`.parallel.meta`).
* **Behavior**: If the process crashes or WiFi dies, re-running the command reads the journal, verifies the file hash of downloaded chunks, and resumes exactly where it left off. No "starting over from 0%".

---

## Installation

### The Simple Way (Homebrew)

On macOS or Linux:

```bash
brew install darkprince558/tap/jend
```

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

### QR Code Mode

The easiest way to share a file with someone on the same network, especially on enterprise or school Wi-Fi where peer-to-peer connections are blocked. Run `--qr` and a QR code appears in your terminal. Anyone can scan it with their phone or laptop camera—the file opens in their browser, no JEND installation needed.

```bash
jend send --qr report.pdf
```

The browser download page shows the file name, size, type, and SHA-256 hash before the download starts, so the receiver can confirm what they are getting. After the download completes, the server shuts itself down automatically.

This also works for text snippets:

```bash
jend send --qr --text "https://internal-link.corp/doc"
```

### Persistent Configuration

Dont want to type flags every time? Save your preferences.

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
| **QR Mode** | `--qr` | Start a local HTTP server and display a QR code. The receiver opens the URL in any browser. No JEND install required on their end. |
| **Incognito** | `--incognito` | Disables history logging and clipboard copying. Use this for sensitive data you don't want tracked locally. |
| **Compression** | `--tar` / `--zip` | Manually force a compression format. JEND usually detects this automatically for directories. |
| **Automation** | `--headless` | Runs without the interactive UI (TUI). Outputs machine-readable logs to stdout for scripts. |
| **S3 Mode** | `--s3` | Upload files to temporary cloud storage (AWS S3) for the receiver to download later. Useful for asynchronous transfers. |
| **Custom Relay** | `--relay-url` | Override the default relay with your own TURN server address. |

**Examples:**

```bash
# Share a file with someone who doesn't have JEND installed (school/work WiFi)
jend send --qr presentation.pptx

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
* `jend config clear-relay` — Reset to default settings.
