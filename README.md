# Pulse

🚀 **Secure file transfer between terminal and phone with a QR code**

No apps. No accounts. End-to-end encrypted. Transfer files instantly with unmatched simplicity and security.

```
$ pulse send document.pdf
```

## Features

- ✅ **Zero-Knowledge Architecture** - Relay server never sees unencrypted data
- ✅ **QR Code Transfer** - Scan and transfer instantly, no setup needed
- ✅ **E2E Encryption** - Military-grade NaCl secretbox (XSalsa20-Poly1305)
- ✅ **Batch Transfers** - Send multiple files in one session
- ✅ **Checksum Verification** - SHA256 integrity checking
- ✅ **Auto-Retry** - Automatic reconnection on failures
- ✅ **Transfer History** - Local history tracking
- ✅ **Desktop Notifications** - Get alerts on completion
- ✅ **Debug Mode** - Verbose logging for troubleshooting
- ✅ **Configurable Chunking** - Custom chunk sizes for optimization

## Install

### Auto-Detection (Recommended)
```bash
curl -sL https://raw.githubusercontent.com/fromjyce/pulse/main/install.sh | sh
```

### From Source
```bash
go install github.com/fromjyce/pulse/cmd/pulse@latest
```

## Usage

### Send file (server → phone)
```bash
pulse send document.pdf
pulse send file1.txt file2.pdf file3.zip  # Batch transfer
```

### Receive file (phone → server)
```bash
pulse receive              # Receive to current directory
pulse receive ~/Downloads  # Receive to specific directory
```

### View transfer history
```bash
pulse history
```

## Advanced Options

```bash
pulse --help

Flags:
  --relay <url>       Relay server URL (default: wss://pulse.relay.app)
  --debug             Enable debug logging for troubleshooting
  --chunk-size <n>    Chunk size in bytes (default: 65536)
  --timeout <d>       Transfer timeout (default: 5m)
  --retries <n>       Connection retries on failure (default: 3)
  --notify            Send desktop notification on completion
```

### Examples

```bash
# Send with debug logging
pulse --debug send config.yaml

# Send with custom chunk size for slow connections
pulse --chunk-size 32768 send large_file.iso

# Receive with notifications
pulse --notify receive ~/Downloads

# Send multiple files with longer timeout
pulse --timeout 10m send file1 file2 file3
```

## How It Works

1. **Key Generation** - CLI generates 32-byte encryption key locally
2. **URL Fragment** - Key embedded in URL fragment (`#...`) — never sent to server
3. **File Encryption** - File encrypted locally, streamed through relay in chunks
4. **Phone Decryption** - Browser decrypts chunks in real-time using URL key
5. **Relay Security** - Relay only sees random encrypted bytes and token
6. **Checksum Verification** - Receiver verifies SHA256 after transfer complete

**Complete zero-knowledge model**: The relay never has access to unencrypted data or encryption keys.

## Self-Hosted Relay

Run your own Pulse relay server:

```bash
docker run -p 8080:8080 ghcr.io/fromjyce/pulse-relay

# Use with custom relay
pulse --relay wss://your-server.com:8080 send file.txt
```

## Security

| Aspect | Implementation |
|--------|-----------------|
| **Encryption** | NaCl secretbox (XSalsa20-Poly1305) |
| **Key Exchange** | URL fragment (never sent to server) |
| **Authentication** | Random single-use tokens, 10-minute expiry |
| **Integrity** | SHA256 checksum verification |
| **Retry Policy** | Exponential backoff (2s, 4s, 6s) |

## What's Different ?

### Protocol Enhancements
- Extended metadata with MIME type detection
- SHA256 checksum verification
- Batch transfer support
- Cancel message handling
- Improved error reporting

### CLI Features
- Debug mode for troubleshooting
- Transfer history tracking
- Desktop notifications
- Configurable timeouts and retries
- Batch file transfer support
- Better progress indicators with speed display

### Code Quality
- Enhanced error messages with diagnostics
- Automatic connection retry with backoff
- Context-based cancellation support
- MIME type detection
- Statistics tracking (speed, duration)
- Improved logging infrastructure

## Transfer History

Pulse maintains a local transfer history at `~/.pulse/history.json`:

```bash
pulse history

📋 Transfer History

Time                | Dir  | File                    | Size    | Speed    | Status
────────────────────────────────────────────────────────────────────────────────────
2024-02-13 14:32:10 | ↑   | document.pdf            | 2.3 MB  | 547 KB/s | ok
2024-02-13 14:28:45 | ↓   | report.xlsx             | 1.2 MB  | 285 KB/s | ok
2024-02-13 14:25:20 | ↑   | config.yaml             | 4.2 KB  | 1.2 MB/s | ok
```

# Contact
If you come across any issues, have suggestions for improvement, or want to discuss further enhancements, feel free to contact me at jaya2004kra@gmail.com. Your feedback is greatly appreciated.

# License
All the code and resources in this repository are licensed under the GNU GENERAL PUBLIC LICENSE. You are free to use, modify, and distribute the code under the terms of this license. However, I do not take responsibility for the accuracy or reliability of the programs.
