# JEND

JEND is a modern, high-speed file transfer tool designed for the command line. It combines the speed of UDP/QUIC with the simplicity of magic wormhole-style transfers. No servers*, no complex setup—just secure, peer-to-peer file sharing.

![Terminal Demo Placeholder](https://via.placeholder.com/800x400?text=JEND+TUI+Demo)

## Why Jend?

*   **Blazing Fast**: Built on **QUIC**, the same protocol powering HTTP/3. Fast connection establishment and no head-of-line blocking.
*   **Secure by Default**: Uses **PAKE** (Password-Authenticated Key Exchange). We never see your files. Encryption keys are generated ephemerally and wiped after transfer.
*   **Smart Handling**: Sending a folder? JEND automatically bundles it into a `.tar.gz`. Receiving one? A simple `--unzip` flag handles extraction.
*   **Automation Ready**: Full headless mode for CI/CD pipelines, cron jobs, and scripts.

---

## Installation

```bash
go install github.com/darkprince558/jend/cmd/jend@latest
```

---

## Quick Start

### 1. Send Anything
Directly send a file or an entire directory.

```bash
# Send a single file
jend send data.csv

# Send a folder (automatically compressed)
jend send my_project/
```
*You'll get a 3-word code like: `amaze-tiger-bravo`*

### 2. Receive Securely
On the other machine, just use the code.

```bash
# Receive the file
jend receive amaze-tiger-bravo

# Receive and automatically unzip
jend --unzip receive amaze-tiger-bravo
```

---

## Power User Features

### Compression Control
Take full control over how your files are packaged.
*   `--zip`: Force zip compression before sending.
    ```bash
    jend --zip send huge_log.txt
    ```
*   `--tar`: Force tar.gz compression.
*   `--unzip`: Receiver flag to automatically extract archives upon arrival.

### Headless & Automation
Perfect for DevOps. Run JEND without the UI in your background scripts.
```bash
# Run silently with a 5-minute timeout
jend --headless --timeout 5m send backup.db
```

### Safety First
*   **Integrity Check**: Every transfer is verified with a SHA-256 checksum to ensure bit-perfect delivery.
*   **Path Sanitization**: Automatic protection against Zip Slip attacks.
*   **Timeout**: Transfers automatically expire if not accepted within the timeout window (default 10m).

### Audit Trail
Keep track of every file you send or receive.
```bash
# View transfer history
jend history

# View detailed proof for a specific transfer
jend history partial-red-panda

# Clear your logs
jend history --clear
```

---

## Coming Soon (Roadmap)

We are actively building the ultimate transfer tool. Here is what's next:

*   **Network Intelligence**:
    *   **Smart Discovery**: Hybrid mDNS (LAN) + P2P Hole Punching + Relay Fallback.
    *   **IPv6 Support**: First-class citizen support.
*   **"Resume Gold"**:
    *   **Auto-Resume**: Interruptions will just pause, not fail.
    *   **Parallel Streaming**: Splitting large files for max bandwidth.
    *   **Bandwidth Throttling**: Limits via `--limit 5MB/s`.
*   **New Modes**:
    *   **Clientless Web Receive**: Receive files directly in a browser.
    *   **Dead Drop**: Encrypted async upload for offline receivers.
    *   **Ghost Mode**: Force relay usage for absolute IP privacy.
    *   **Dead Drop**: Encrypted async upload for offline receivers.
    *   **Ghost Mode**: Force relay usage for absolute IP privacy.


---
*Note: Currently defaults to direct connection (localhost/IP). Use in trusted networks or tunnel accordingly.*
