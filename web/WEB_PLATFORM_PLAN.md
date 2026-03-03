# JEND Web Platform - Implementation Plan

This is the comprehensive architectural and feature blueprint to evolve JEND from a CLI-only tool into a universal, hosted P2P file sharing platform (similar to Snapdrop, WebWormhole, or LocalSend, but with first-class CLI support).

## Core Vision

A user types `jend.app` (or your chosen domain) into any browser on any device. They are immediately placed in a session. They can drag-and-drop files to send them directly to other local users, or enter a unique code to connect with remote users (including users using the JEND CLI).

## User Review Required
>
> [!IMPORTANT]
> This is a massive feature epic. We need to decide on the Frontend Tech Stack (React/Next.js, Svelte, Vue, or Vanilla JS) and where to host the Signaling Server (AWS, Fly.io, Render, etc.). Please review the proposed architecture and let me know your preferences!

## Architectural Components

### 1. The Web Frontend (Browser Client)

* **Tech Stack**: Next.js (React) or SvelteKit with TailwindCSS for beautiful, dynamic UI.
* **Core Tech**: `WebRTC RTCDataChannel` for browser-to-browser and browser-to-CLI peer-to-peer file routing.
* **PWA (Progressive Web App)**: App must include a Web App Manifest and Service Workers so users can "Install" it on iOS/Android and use the OS native "Share" menu to send elements directly to the JEND Web App.

### 2. The Global Signaling Server

* **Tech Stack**: Go (can live in your current JEND repo under `cmd/signaling/`).
* **Purpose**: Browsers cannot use mDNS (local network broadcast) for discovery. They need a central WebSocket server to exchange initial SDP offers and ICE candidates.
* **Mechanism**: Users are assigned a random word-pair (`brave-tiger`) or short numeric code. The signaling server matches peers with the same code and orchestrates the WebRTC connection.

### 3. P2P Traversal Infrastructure (STUN / TURN)

* **STUN**: We will use public STUN servers (like Google's) to let peers discover their public IP addresses.
* **TURN**: To guarantee 100% transfer success (for users trapped behind strict corporate symmetric NATs), we should deploy an open-source TURN server (like `coturn`). If P2P fails, traffic routes through the relay.

## Proposed Features

### Core Web Features

* **Seamless Web-to-Web** P2P file transfer (no file size limits, as it does not touch a server).
* **Text / Snippet Sharing**: Share clipboard text or URLs.
* **QR Handshake**: The desktop web app shows a QR code; scanning it on a phone instantly pairs the two devices without typing a code.
* **Interactive Approvals**: Beautiful incoming file dialogs (Accept / Reject).

### CLI ↔ Web Interoperability

* **Command**: `jend send <file> --web`. The CLI connects to the global Signaling server and outputs a code (e.g., `Code: 4812`).
* **Receive**: The user opens the web app, types `4812`, and the CLI sends the file directly to the browser via WebRTC.
* **Vice Versa**: A browser user generates a code, and the CLI user types `jend receive 4812`.

### Advanced / Premium Features (The "More Powerful" aspects)

* **Multi-user Dropzones (Rooms)**: Enter a shared code (e.g., `class-project`) and 5 people are in a room. Anyone can drop a file, and anyone can download it.
* **E2E Encrypted Chat**: Simple local chat built into the Dropzone rooms.
* **Web Directory Streaming**: Using the File System Access API, share an entire folder tree from the browser for others to browse and selectively download files, rather than pushing a zip file.
* **Wormhole-style Fallback (Optional)**: If peers aren't online at the same time, encrypt the file locally in the browser, upload the encrypted blob to an S3 bucket, and give the user a link. When the receiver opens the link, the file is decrypted in the browser and the server deletes the blob.

## Proposed Changes (Code Structure)

### `web/` Frontend Codebase

Create a new directory for the modern web application.

#### [NEW] `web/package.json`

#### [NEW] `web/src/pages/index.tsx`

#### [NEW] `web/src/lib/webrtc.ts` (WebRTC negotiation logic)

### `cmd/signaler/` and `internal/signaling/`

#### [NEW] `cmd/signaler/main.go`

The globally hosted WebSocket backend for matching Web and CLI users using codes.

#### [NEW] `internal/signaling/hub.go`

Manages active WebSocket connections, rooms, and message routing.

### `internal/transport/`

#### [MODIFY] `internal/transport/webrtc.go`

Extend the existing CLI WebRTC implementation to securely communicate via the global signaling server in addition to the local mDNS discovery.

## Verification Plan

### Automated Tests

* **Signaling Server Tests**: Write Go unit tests simulating multiple WebSocket clients connecting, joining rooms, and successfully exchanging SDP offers/answers.
* **Web UI Tests**: Playwright or Cypress E2E tests for the web interface (simulating two browser instances doing a file transfer).
* **Cross-compatibility Tests**: A Go integration test that runs the CLI, generates a code, connects a headless browser via Puppeteer, enters the code, and verifies the file transfers from CLI to Browser.

### Manual Verification

* Deploy the frontend to Vercel/Netlify.
* Deploy the signaling server to Fly.io/AWS.
* Open the web app on an iPhone (via cellular) and a Mac (via WiFi).
* Ensure file sending works seamlessly across the internet.
* Run `jend send` on the CLI and receive it successfully on the hosted iPhone web app.
