#!/bin/sh
set -eu

TOKEN="${1:-${RC_ENROLL_TOKEN:-}}"
STATE_DIR="${RC_STATE_DIR:-$HOME/.config/ohrats-rc}"

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
LEGACY_BIN="$DIR/ohrats-relay"
LEGACY_STATE="$HOME/.config/ohrats-relay"
mkdir -p "$DIR"

URL="https://rc.ohrats.party/downloads/ohrats-rc-${OS}-${ARCH}"
TMP="$DIR/.ohrats-rc.$$"
trap 'rm -f "$TMP"' EXIT HUP INT TERM
curl -fsSL "$URL" -o "$TMP"
if [ ! -s "$TMP" ] || [ "$(head -c 1 "$TMP" || true)" = "<" ]; then
  echo "downloaded ohrats-rc is not a valid binary" >&2
  exit 1
fi
chmod 755 "$TMP"

# Greenfield Relay → RC cutover. Remove only an installation that identifies
# itself as the former OhRats Relay Node; do not preserve legacy state/env.
# Do this only after the replacement binary has downloaded and validated.
if [ -x "$LEGACY_BIN" ] && "$LEGACY_BIN" version 2>/dev/null | grep -q '^OhRats Relay Node '; then
  if command -v pkill >/dev/null 2>&1; then
    pkill -TERM -x ohrats-relay 2>/dev/null || true
    sleep 1
  fi
  rm -f "$LEGACY_BIN"
  rm -rf "$LEGACY_STATE"
  echo "removed obsolete OhRats Relay installation"
fi

mv "$TMP" "$DIR/ohrats-rc"
trap - EXIT HUP INT TERM
echo "installed $DIR/ohrats-rc"
if [ -n "$TOKEN" ]; then
  "$DIR/ohrats-rc" enroll "$TOKEN"
  echo ""
  echo "OhRats RC Node installed and enrolled."
elif [ -s "$STATE_DIR/device.json" ]; then
  echo ""
  echo "OhRats RC Node updated."
else
  echo ""
  echo "OhRats RC Node installed."
  echo "enroll: ohrats-rc enroll ENROLLMENT_TOKEN"
fi
echo "run:    ohrats-rc run"
echo "help:   ohrats-rc --help"
