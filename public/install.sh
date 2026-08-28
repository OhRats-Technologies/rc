#!/bin/sh
set -eu

TOKEN="${1:-${RC_ENROLL_TOKEN:-}}"
SERVER="${2:-${RC_URL:-}}"
STATE_DIR="${RC_STATE_DIR:-$HOME/.config/rc}"
API="${RC_RELEASE_API:-https://api.github.com/repos/OhRats-Technologies/rc/releases/latest}"

case "$SERVER" in
  ""|http://*|https://*) ;;
  *) echo "RC server URL must use http or https" >&2; exit 1 ;;
esac

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

NAME="rc-${OS}-${ARCH}.tar.gz"
DIR="$HOME/.local/bin"
TMPDIR_PATH="${TMPDIR:-/tmp}/rc-install.$$"
mkdir -p "$DIR" "$TMPDIR_PATH"
trap 'rm -rf "$TMPDIR_PATH"' EXIT HUP INT TERM

curl -fsSL -H 'Accept: application/vnd.github+json' -H 'User-Agent: rc-installer' "$API" -o "$TMPDIR_PATH/github-release.json"
ASSET=$(tr -d '\n\r\t ' < "$TMPDIR_PATH/github-release.json" | awk -v name="$NAME" 'BEGIN { RS="\\},\\{" } index($0, "\"name\":\"" name "\"") { print; exit }')
test -n "$ASSET" || { echo "release does not contain $NAME" >&2; exit 1; }
URL=$(printf '%s\n' "$ASSET" | sed -n 's/.*"browser_download_url":"\([^"]*\)".*/\1/p')
DIGEST=$(printf '%s\n' "$ASSET" | sed -n 's/.*"digest":"\([^"]*\)".*/\1/p')
EXPECTED=${DIGEST#sha256:}
case "$URL" in
  https://github.com/OhRats-Technologies/rc/releases/download/*) ;;
  *) echo "release asset has an invalid download URL" >&2; exit 1 ;;
esac
case "$DIGEST" in
  sha256:????????????????????????????????????????????????????????????????) ;;
  *) echo "release asset is missing its GitHub SHA-256 digest" >&2; exit 1 ;;
esac
case "$EXPECTED" in
  *[!0-9a-fA-F]*) echo "release asset has an invalid SHA-256 digest" >&2; exit 1 ;;
esac

curl -fsSL "$URL" -o "$TMPDIR_PATH/$NAME"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TMPDIR_PATH/$NAME" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$TMPDIR_PATH/$NAME" | awk '{print $1}')
else
  echo "sha256sum or shasum is required" >&2
  exit 1
fi
test "$ACTUAL" = "$EXPECTED" || { echo "downloaded RC checksum mismatch" >&2; exit 1; }

test "$(tar -tzf "$TMPDIR_PATH/$NAME")" = "rc" || {
  echo "release archive must contain only rc" >&2; exit 1;
}
tar -xzf "$TMPDIR_PATH/$NAME" -C "$TMPDIR_PATH"
test -f "$TMPDIR_PATH/rc" || { echo "release archive does not contain rc" >&2; exit 1; }
chmod 0755 "$TMPDIR_PATH/rc"
"$TMPDIR_PATH/rc" version | grep -Eq '^RC [0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$' || {
  echo "downloaded rc did not report a valid version" >&2; exit 1;
}
cp "$TMPDIR_PATH/rc" "$DIR/.rc-install.$$"
chmod 0755 "$DIR/.rc-install.$$"
mv "$DIR/.rc-install.$$" "$DIR/rc"

echo "installed $DIR/rc"
if "$DIR/rc" upgrade --help >/dev/null 2>&1; then
  "$DIR/rc" upgrade
fi
if [ -n "$TOKEN" ]; then
  if [ -n "$SERVER" ]; then
    "$DIR/rc" enroll "$TOKEN" --url "$SERVER"
  else
    "$DIR/rc" enroll "$TOKEN"
  fi
fi
if [ -s "$STATE_DIR/device.json" ]; then
  if "$DIR/rc" service install; then
    echo "node:   running in the background"
  else
    echo "warning: could not start the background service" >&2
    echo "run:    rc run" >&2
  fi
else
  echo "enroll: rc enroll ENROLLMENT_TOKEN"
fi
echo "help:   rc --help"
