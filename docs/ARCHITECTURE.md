# JEND Architecture

JEND is designed to be a resilient, secure peer-to-peer file transfer system. This document outlines the core technical components that allow JEND to guarantee delivery even on restrictive or hostile networks.

## 1. Cryptography and Security Model

JEND assumes that the network is always compromised and that an attacker may control the relay servers or the local Wi-Fi. It uses a **Zero-Trust** security model.

### Password-Authenticated Key Exchange (PAKE)

When a sender generates a 3-word transfer code (e.g., `happy-delta-seven`), the receiver types this code in. However, the code itself is **never sent over the network**, preventing a Man-in-the-Middle (MitM) attacker who intercepts the connection from learning the password.

JEND uses the **SPAKE2** protocol (an Augmented PAKE). The sender and receiver mathematically prove to each other that they hold the same secret code. Only if the proof succeeds do they derive a shared cryptographic key.

### Key Derivation and Hardening

Because users type short 3-word codes, the entropy is relatively low. To prevent an attacker from capturing the handshake and brute-forcing the code offline, JEND hardens the key derivation using **Argon2id**.

- **Parameters**: 64MB Memory, 3 Iterations.
- **Result**: Even if an attacker captures the traffic, guessing a single code takes significant CPU and memory time, rendering brute-force attacks economically unviable.

### Authenticated Encryption

Once the SPAKE2 handshake completes, both parties share a strong `EncryptionKey`. All file data and metadata sent between peers is encrypted using **AES-256-GCM**.

- **Confidentiality**: The relay server, ISP, and internal network admins only see opaque, randomized bytes.
- **Integrity**: Any tampered or injected packets will fail the GCM authentication tag check and be immediately dropped.

## 2. Network Traversal (ICE, STUN, TURN)

Direct P2P connectivity over the internet is blocked by Network Address Translation (NAT) and stateful firewalls.

### Local Discovery (mDNS)

When both clients are on the same local network, JEND broadcasts an `mDNS` (Multicast DNS) packet. If the peers discover each other locally, they transfer the file directly over the LAN, ignoring the internet entirely.

### Cloud Registry & Signaling

If mDNS fails (e.g., multicast is blocked, or the users are in different countries), the peers use a lightweight signaling server (DynamoDB + Lambda) to exchange connection details. Note: The signaling server ONLY sees base64-encoded encrypted envelopes. It does not know the transfer code or the file content.

### NAT Hole Punching (STUN)

To connect across the internet:

1. Both clients ping a STUN server to learn their public-facing IP address and port.
2. They swap these public IP addresses via the signaling server.
3. They simultaneously send UDP packets to each other's public IP port, tricking their respective NAT firewalls into "punching a hole" to allow the incoming traffic.

### The Relay Fallback (TURN)

Some enterprise networks feature **Symmetric NATs** or aggressive firewalls that block all raw UDP hole-punching attempts. If STUN fails, JEND falls back to a **TURN Server** (Traversal Using Relays around NAT).

- JEND provisions an AWS EC2 `coturn` instance running on TCP Port 443.
- To the firewall, the traffic looks like a standard secure HTTPS connection.
- The TURN server blindly relays the AES-GCM encrypted packets between the sender and receiver.

## 3. The Transport Protocol: QUIC

JEND is built on top of the **QUIC** protocol (RFC 9000), rather than standard TCP.

- **Head-of-Line Blocking**: In TCP, if packet #4 drops, the entire connection halts until packet #4 is retransmitted—even if packets #5 through #100 arrived perfectly. QUIC multiplexes streams. In JEND, file chunks are sent over independent streams. If a chunk drops, only that chunk is delayed; the rest of the transfer continues at full speed.
- **Congestion Control**: QUIC uses advanced congestion control (like BBR) to rapidly scale up to available bandwidth on lossy connections.
- **Connection Migration**: If your laptop switches from Wi-Fi to a cellular hotspot mid-transfer, QUIC can transparently migrate the connection without interrupting the download.

## 4. WebQR Architecture

To support users without the CLI installed, JEND includes a "Browser Receive" mode.

- **The Problem**: Browsers cannot run raw QUIC or raw UDP sockets. They can only use WebSockets or WebRTC.
- **The Solution**: When you run `jend send --qr`, JEND spins up an embedded HTTP server (in Local mode) or acts as a WebRTC DataChannel peer (in Cloud mode).
- **In Cloud Mode**: The sender CLI negotiates a WebRTC connection with the receiver's Browser. The browser fetches dynamic TURN credentials from the AWS API Gateway to ensure it can traverse strict enterprise firewalls via TCP port 443. The payload is chunked and streamed over the WebRTC DataChannel directly into browser memory (or the File System Access API), avoiding intermediate storage.
