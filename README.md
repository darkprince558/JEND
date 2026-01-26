# JEND

JEND is a high-performance, peer-to-peer file transfer tool written in Go. It allows you to move files securely between computers—regardless of whether they are on the same WiFi or behind strict corporate firewalls—without configuring servers or opening ports.

It was built to solve the fragility of existing tools (like `scp` or `rsync`) in unstable network conditions, using modern transport protocols to guarantee delivery.

## Architecture & Internals

I built JEND to handle the edge cases that break other transfer tools. Here is how it works under the hood:

### 1. Transport Layer: QUIC (UDP)

Instead of standard TCP, JEND runs over **QUIC** (the protocol powering HTTP/3).

* **Why?** TCP suffers from head-of-line blocking; if one packet is lost, the entire connection halts. QUIC multiplexes streams, so if a packet drops on one stream, the others keep moving. This effectively saturates available bandwidth on lossy networks (like public WiFi).

### 2. Security: Augmented PAKE + Argon2id

The most critical part of a transfer tool is the "handshake". How do two strangers trust each other without a pre-shared key?

* **Protocol**: I implemented a **Password-Authenticated Key Exchange (PAKE)**. This mathematically proves both parties know the code (e.g. `correct-horse-battery`) without ever sending the code over the wire.
* **Hardening**: Standard PAKE is vulnerable to GPU brute-forcing if the password is simple. I upgraded the key derivation to **Argon2id** (Time=3, Memory=64MB), forcing any attacker to spend ~50-100ms of CPU time per guess. This makes the 3-word code mathematically secure for the duration of the transfer window.

### 3. Network Traversal: ICE & Custom Relays

Direct P2P connectivity is blocked by most NATs.

* **Discovery**: JEND first attempts local mDNS discovery (IPv4/IPv6).
* **Hole Punching**: If local discovery fails, it uses **ICE** (Interactive Connectivity Establishment) to punch holes through NATs.
* **Configurable Relays**: For strict networks, I implemented a **"Bring Your Own Relay"** system. You can point JEND at any standard TURN server (e.g., a free Oracle Cloud instance) to route traffic when P2P is impossible. Secure, private routing without vendor lock-in.

### 4. Reliability: State-Machine Resumption

Transfers over 100MB fail often.

* **Mechanism**: JEND maintains a persistent state journal on disk (`.parallel.meta`).
* **Behavior**: If the process crashes or WiFi dies, re-running the command reads the journal, verifies the file hash of downloaded chunks, and resumes exactly where it left off. No "starting over from 0%".

---

## Installation

Download the latest binary for your OS from the [Releases](#) page, or build from source:

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
# Open 16 parallel QUIC streams
jend receive --concurrency 16 happy-delta-seven
```
