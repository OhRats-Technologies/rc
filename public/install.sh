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

NAME="rc-${OS}-${ARCH}"
RELEASE_BASE="https://github.com/OhRats-Technologies/rc/releases/latest/download"
URL="$RELEASE_BASE/$NAME"
TMP="$DIR/.rc.$$"
SUMS="$DIR/.rc-sums.$$"
trap 'rm -f "$TMP" "$SUMS"' EXIT HUP INT TERM
curl -fsSL "$URL" -o "$TMP"
curl -fsSL "$RELEASE_BASE/SHA256SUMS" -o "$SUMS"
if [ ! -s "$TMP" ] || [ "$(head -c 1 "$TMP" || true)" = "<" ]; then
  echo "downloaded rc is not a valid binary" >&2
  exit 1
fi
EXPECTED=$(awk -v name="$NAME" '$2 == name { print $1 }' "$SUMS")
if [ -z "$EXPECTED" ]; then
  echo "release checksum missing for $NAME" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TMP" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$TMP" | awk '{ print $1 }')
else
  echo "sha256sum or shasum is required" >&2
  exit 1
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "downloaded rc checksum mismatch" >&2
  exit 1
fi
chmod 755 "$TMP"
if ! "$TMP" version 2>/dev/null | grep -Eq '^RC [0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "downloaded rc did not report a valid version" >&2
  exit 1
fi
mv "$TMP" "$DIR/rc"
trap - EXIT HUP INT TERM
rm -f "$SUMS"
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
