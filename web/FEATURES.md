# JEND Web Platform: Comprehensive Feature List & Specification

This document outlines the desired feature set for `jend.app` to make it a top-tier file-sharing platform, taking inspiration from the existing JEND CLI, LocalSend, and Snapdrop.

## 1. Core Transfer Modalities
* **Device-to-Device within Local Network (Snapdrop/LocalSend style)**
   * Auto-discovery of other devices on the same Wi-Fi network. Since mDNS isn't available in browsers, this is achieved by the signaling server grouping clients by their public IP address.
   * "Radar" or "Network Map" view showing nearby devices graphically.
   * 1-click send to a discovered device without needing a 6-word transfer code.
* **Internet-Wide Global Transfer (Wormhole/JEND CLI style)**
   * Generate Human-Readable Transfer Codes (e.g., `happy-delta-seven`).
   * Enter a Transfer Code via input field to pair with a device anywhere in the world.
* **Cross-Platform Compatibility (CLI <-> Web)**
   * Full WebRTC interoperability between the browser and the JEND Go CLI.
   * Send from Terminal -> Receive in Browser.
   * Send from Browser -> Receive in Terminal.

## 2. User Experience (UX) & Interactions
* **Premium Aesthetics & "Wow" Factor**
   * Sleek glassmorphism or minimalist cards on a dynamic gradient/mesh background. Design built in Figma.
   * Fluid micro-animations for file selection, upload progress, and completion states. 
   * Interactive hover states and subtle glow effects to make the app feel alive.
   * Theme Switcher matching CLI themes: Dracula, Nord, Dark/Light, etc.
* **Drag & Drop Dashboard**
   * Immersive drag-and-drop zone that highlights or encompasses the screen when dragging a file into the browser window.
   * Visual file previews (image thumbnails, parsed icons for PDFs, Zips, etc.).
* **Real-time Transfer Analytics**
   * Circular or linear progress bars showing exactly where the transfer is at.
   * Live metrics: Transfer speed (MB/s) and Estimated Time Remaining (ETA).
   * "Confetti" or satisfying success animation upon successful completion.

## 3. Advanced File Handling
* **Multi-File & Directory Support**
   * Select multiple files or entire folders.
   * Auto-compression using client-side JS (e.g. `fflate` or `JSZip`) to create a `.zip` entirely on the client-side prior to sending. This mirrors the CLI's `--zip` flag behavior.
* **Raw Text & Snippet Sharing**
   * Dedicated "Text" or "Clipboard" tab to beam strings, links, or JSON quickly.
   * 1-click "Copy to Clipboard" on the receiving end.
* **Auto-Accept & Safe Saving**
   * Optionally toggle "Auto-Download" to automatically save incoming files without prompting (like LocalSend's Quick Save).
   * Handle large files up to several GBs using the browser streams API (or `indexedDB` if needed) to prevent memory crashes.

## 4. Security & Privacy (The "No-Trust" Model)
* **True Peer-to-Peer (WebRTC)**
   * Files are never stored on a centralized server. They traverse WebRTC Data Channels.
   * Fallback to TURN servers only if symmetric NATs block direct connection (all data remains fully encrypted during relay).
* **Incognito Mode**
   * Option to disable browser-local transfer history log (matches CLI `--incognito`).
* **Optional Future E2EE (Wormhole style)**
   * Generate WebCrypto AES-GCM keys on the sender side, embed the key in the URL anchor (`jend.app/#key`), and rely on the signaling server just for pairing. The server never sees the actual key.

## 5. Utility & Quality of Life
* **Transfer History / Audit Log**
   * A localized ledger stored in `localStorage` showing past transfers, dates, sizes, and peer names.
* **QR Code Pairing (Mobile-First)**
   * Auto-generate a QR Code encoding the transfer URL (e.g., `jend.app/receive/happy-delta-seven`).
   * Mobile devices can just scan the screen to instantly connect and download.
* **PWA (Progressive Web App)**
   * Installable to iOS/Android home screens for a native-app feel.
   * Registers as a Share Target (e.g., "Share to JEND" directly from iOS Photos/Files app).

---

## Implementation Phasing Strategy

* **Phase 1 (MVP)**: 
  * Send/Receive single files via global code. 
  * Basic WebRTC data channel, manual signaling integration with Go backend.
* **Phase 2 (Parity)**: 
  * UI Polish based on Figma designs. 
  * Multi-file auto-ZIP, Text snippet sharing. 
  * Ensure full CLI Interop is flawless.
* **Phase 3 (Magic)**: 
  * Local IP auto-discovery (The Snapdrop experience). 
  * QR codes, Local History Log.
* **Phase 4 (Ecosystem)**: 
  * PWA integration and Offline-first caching of assets. 
  * Native Share extensions.
