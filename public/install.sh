#!/bin/sh
set -eu

TOKEN="${1:-${RELAY_ENROLL_TOKEN:-}}"
if [ -z "$TOKEN" ]; then
  echo "usage: curl -fsSL https://relay.ohrats.party/install.sh | sh -s -- ENROLLMENT_TOKEN" >&2
  exit 1
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported operating system: $OS" >&2; exit 1 ;;
esac

DIR="$HOME/.local/bin"
mkdir -p "$DIR"
URL="https://relay.ohrats.party/downloads/relay-agent-${OS}-${ARCH}"
curl -fsSL "$URL" -o "$DIR/relay-agent"
chmod 755 "$DIR/relay-agent"
echo "installed $DIR/relay-agent"
exec "$DIR/relay-agent" --enroll "$TOKEN"
