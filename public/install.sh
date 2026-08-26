#!/bin/sh
set -eu

TOKEN="${1:-${RC_ENROLL_TOKEN:-}}"
STATE_DIR="${RC_STATE_DIR:-$HOME/.config/rc}"

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

URL="https://rc.ohrats.party/downloads/rc-${OS}-${ARCH}"
TMP="$DIR/.rc.$$"
trap 'rm -f "$TMP"' EXIT HUP INT TERM
curl -fsSL "$URL" -o "$TMP"
if [ ! -s "$TMP" ] || [ "$(head -c 1 "$TMP" || true)" = "<" ]; then
  echo "downloaded rc is not a valid binary" >&2
  exit 1
fi
chmod 755 "$TMP"
mv "$TMP" "$DIR/rc"
trap - EXIT HUP INT TERM
echo "installed $DIR/rc"
if [ -n "$TOKEN" ]; then
  "$DIR/rc" enroll "$TOKEN"
  echo ""
  echo "RC Node installed and enrolled."
elif [ -s "$STATE_DIR/device.json" ]; then
  echo ""
  echo "RC Node updated."
else
  echo ""
  echo "RC Node installed."
  echo "enroll: rc enroll ENROLLMENT_TOKEN"
fi
if [ -s "$STATE_DIR/device.json" ]; then
  if "$DIR/rc" service install; then
    echo "node:   running in the background"
  else
    echo "warning: could not start the background service" >&2
    echo "run:    rc run" >&2
  fi
fi
echo "help:   rc --help"
