#!/bin/sh
set -eu

TOKEN="${1:-${RELAY_ENROLL_TOKEN:-}}"
STATE_DIR="${RELAY_STATE_DIR:-$HOME/.config/ohrats-relay}"

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
URL="https://relay.ohrats.party/downloads/ohrats-relay-${OS}-${ARCH}"
TMP="$DIR/.ohrats-relay.$$"
trap 'rm -f "$TMP"' EXIT HUP INT TERM
curl -fsSL "$URL" -o "$TMP"
if [ ! -s "$TMP" ] || [ "$(head -c 1 "$TMP" || true)" = "<" ]; then
  echo "downloaded ohrats-relay is not a valid binary" >&2
  exit 1
fi
chmod 755 "$TMP"
mv "$TMP" "$DIR/ohrats-relay"
trap - EXIT HUP INT TERM
echo "installed $DIR/ohrats-relay"
if [ -n "$TOKEN" ]; then
  "$DIR/ohrats-relay" enroll "$TOKEN"
  echo ""
  echo "OhRats Relay Node installed and enrolled."
elif [ -s "$STATE_DIR/device.json" ]; then
  echo ""
  echo "OhRats Relay Node updated."
else
  echo ""
  echo "OhRats Relay Node installed."
  echo "enroll: ohrats-relay enroll ENROLLMENT_TOKEN"
fi
echo "run:    ohrats-relay run"
echo "help:   ohrats-relay --help"
