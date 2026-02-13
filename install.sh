#!/bin/sh
set -e

# Check if the user is root
if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
else
    SUDO="sudo"
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

URL="https://github.com/fromjyce/pulse/releases/latest/download/pulse-${OS}-${ARCH}"

echo "Downloading Pulse for ${OS}-${ARCH}..."
curl -sL "$URL" -o /tmp/pulse
chmod +x /tmp/pulse

# Use the dynamic variable here
$SUDO mv /tmp/pulse /usr/local/bin/pulse

echo "Installed! Run: pulse send <file>"
