# JEND CLI Reference

This document outlines the commands and flags available in the JEND CLI.

## Global Flags

These flags can be appended to almost any command.

* `--help`, `-h`: Show help for a command.

---

## Interactive Launch

```bash
jend
```

Running `jend` without any arguments launches the interactive TUI (Terminal UI) wizard. You can browse for files, type text snippets, or toggle features visually.

---

## Send Command

Send a file, folder, or text snippet to a peer.

```bash
jend send <path> [flags]
```

### Examples

* `jend send my_project.zip` (Send a single file)
* `jend send folder/ data.sql` (Send multiple files/folders; JEND auto-zips them)
* `cat error.log | jend send` (Send generic text from STDIN)

### Flags

#### Transfer Modes

* `--s3`: Uploads the file to an encrypted, temporary cloud bucket instead of a live P2P transfer. The receiver can download it at their leisure. Best for async workflows or very large files that fail over P2P.
* `--qr`: Generates a URL and prints a QR code in the terminal. The receiver can scan the code to download the file directly in their web browser without installing JEND.

#### QR Specific Flags

* `--qr-mode <local|cloud>`:
  * `local` (Default): Runs a lightweight web server on your machine. The receiver must be on the same Wi-Fi network.
  * `cloud`: Uses WebRTC to punch through the internet. The receiver can be located anywhere in the world.
* `--qr-limit <int>`: (Default: 1). The number of times the QR link can be downloaded before it automatically expires.
* `--qr-expire <duration>`: (Default: 1h). How long the QR code remains valid. Valid units: `m`, `h` (e.g., `30m`, `12h`).

#### Privacy

* `--incognito`: Enables stealth mode. Disables saving the transfer to the local audit log (`jend history`) and prevents JEND from automatically copying the transfer code to your clipboard.
* `--no-clipboard`: Prevents JEND from copying the generated code to your clipboard.
* `--no-history`: Prevents JEND from recording the transfer in the local audit log.

#### Compression

By default, sending a single file does not compress it. Sending multiple items auto-compresses them into a ZIP archive.

* `--zip`: Forces the payload to be compressed into a `.zip` file on-the-fly, even if it's just a single file.
* `--tar`: Forces the payload to be compressed into a `.tar.gz` archive.

---

## Receive Command

Receive a file or text snippet using a transfer code.

```bash
jend receive <code> [flags]
```

### Examples

* `jend receive happy-delta-seven`
* `jend receive 4aF92b`

### Flags

* `--out <path>`: Specifies exactly where to save the received file. By default, JEND saves files in the current working directory, avoiding naming collisions.
  * Example: `jend receive <code> --out ~/Downloads/my_file.zip`
* `--incognito`: Disables saving the transfer in the local audit log.

---

## Configuration Commands

Manage your global JEND configuration preferences.

```bash
jend config <subcommand>
```

### Subcommands

* `jend config set-relay`: Interactive prompt to configure a custom TURN/STUN relay server for traversing impossible firewalls.
* `jend config clear-relay`: Resets network traversal to JEND's default public/cloud infrastructure.
* `jend profile`: Displays your current configuration, theme, and network settings.

---

## Theme Command

Launch the visual interactive theme picker.

```bash
jend theme
```

Allows you to preview and select different color palettes for the Terminal UI. Available themes include: Default, Dark, Light, Dracula, Nord, Solarized, Catppuccin, Ocean, and Matrix.

---

## History Command

View a local audit log of all your past sent and received transfers.

```bash
jend history [flags]
```

### Flags

* `--headless`: Prints the audit log to STDOUT as a text table instead of launching the interactive TUI viewer. Useful for piping logs into grep or saving to a file.

---

## Version Command

Print the current version of the JEND CLI.

```bash
jend version
```
