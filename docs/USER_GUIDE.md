# JEND User Guide

Welcome to the JEND User Guide. While the CLI reference gives you a dictionary of commands and flags, this guide is designed to show you how to actually use JEND in the real world.

Below are some common scenarios and the exact commands you need to seamlessly transfer files and text.

## Scenario 1: The quickest way to send a file

**Goal:** You have a file on your laptop and want to send it to a friend's laptop right now.

1. **Sender:** Open your terminal and run:

   ```bash
   jend send my_video.mp4
   ```

   *JEND will print a 3-word code, like `apple-brave-cat`, and copy it to your clipboard.*

2. **Receiver:** Your friend opens their terminal and types:

   ```bash
   jend receive apple-brave-cat
   ```

**Result:** The file transfers securely peer-to-peer. If you are on the same Wi-Fi, it transfers at local network speeds. If you are across the world, it seamlessly punches through the internet.

## Scenario 2: Sending to (or receiving from) a phone

**Goal:** You want to send a file to your iPhone or Android, or you want to easily upload a photo from your phone to your PC, without installing JEND on the phone.

### To send files FROM your PC TO your phone

```bash
jend send my_document.pdf --qr
```

* **What happens:** JEND prints a giant QR code in your terminal.
* **Next step:** Scan the QR code with your phone's camera. Safari or Chrome will open, and the file will immediately start downloading to your phone.

### To receive files FROM your phone TO your PC

```bash
jend receive
```

* **What happens:** The JEND interactive menu appears. Use your arrow keys to select `Scan QR (Upload from Phone)`.
* **Next step:** JEND spins up a secure, temporary web server and prints a QR code. Scan it with your phone, and a clean web interface will appear on your screen allowing you to select photos or type text to send directly to your computer.

## Scenario 3: Sending entire folders

**Goal:** You have a project folder with hundreds of files and subdirectories.

```bash
jend send ./my_project_folder
```

* **What happens:** You don't need to manually zip anything. JEND automatically detects that it is a directory, compresses it into an archive on the fly, and sends it as one solid stream.

## Scenario 4: The async transfer (Cloud Mode)

**Goal:** You want to send a file to a coworker, but they are offline or in a meeting. You can't leave your terminal running all day waiting for them.

```bash
jend send quarterly_report.pdf --s3
```

* **What happens:** Instead of waiting for a P2P connection, JEND encrypts the file and uploads it to a secure, temporary cloud bucket. It gives you a 6-character code (like `A1b2C3`).
* **Next step:** Send that short code to your coworker on Slack. Whenever they are ready, they run `jend receive A1b2C3` and download the file.

## Scenario 5: Sending text and logs (Piping)

**Goal:** You have an error log or a long URL that you want to quickly shoot over to another machine.

```bash
cat error.log | jend send
```

* **What happens:** JEND detects that you piped data into it. It grabs the text from standard input (STDIN) and generates a transfer code.
* **Receiver:** When the receiver types `jend receive <code>`, the text will print out directly into their terminal, and JEND will automatically copy it to their system clipboard.

## Scenario 6: Incognito and stealth transfers

**Goal:** You are transferring a sensitive configuration key and you do not want any trace of it left on your machine. You don't want it saved in the JEND history log, and you don't want the secret code sitting in your clipboard.

```bash
jend send secret_keys.txt --incognito
```

* **What happens:** JEND disables the automatic clipboard copy feature and it will completely bypass the local audit log (`jend history`). Once the transfer is done, it is gone permanently.

## Scenario 7: Overriding the save location

**Goal:** You have a specific script running that expects a downloaded payload to land in `/tmp/payloads/`.

```bash
jend receive happy-delta-seven --out /tmp/payloads/
```

* **What happens:** Normally, JEND saves the file in the directory where you ran the command. The `--out` flag lets you force the save destination, which is perfect for automation scripts.
