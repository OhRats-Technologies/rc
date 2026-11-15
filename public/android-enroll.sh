#!/bin/sh
# Enroll an Android device in RC through Termux and proot.
#
# The native RC kernel links Wasmtime through cgo and RC has no Android
# service manager, so rc runs as a Linux/arm64 binary inside a Debian rootfs
# under Termux proot. Run this from Termux:
#
#   sh -c "$(curl -fsSL <server>/android-enroll.sh)" -- <token> <server> <name>
#
# Requires Termux with proot. Idempotent: reuses an existing rootfs and skips
# enrollment when a device identity already exists.
set -eu
umask 077

TOKEN="${1:-${RC_ENROLL_TOKEN:-}}"
SERVER="${2:-${RC_URL:-}}"
NAME="${3:-${RC_DEVICE_NAME:-}}"
[ -n "$TOKEN" ] || { echo "enrollment token required" >&2; exit 1; }
[ -n "$SERVER" ] || { echo "server URL required" >&2; exit 1; }

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
ROOTFS="${RC_ANDROID_ROOTFS:-$HOME/debian}"
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) RC_ARCH=arm64 ;;
  x86_64|amd64) RC_ARCH=amd64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

say() { printf '%s\n' "$*" >&2; }

command -v proot >/dev/null 2>&1 || {
  say "proot is required: pkg install proot"
  exit 1
}

# proot needs the glibc loader shim; without it every guest execve fails ENOENT
# because Android has no /lib/ld-linux-* at the guest path.
LOADER="$PREFIX/libexec/proot/loader"
[ -f "$LOADER" ] || { say "missing proot loader: $LOADER" >&2; exit 1; }

PROOT_ARGS="-r $ROOTFS -b /dev -b /proc -b /sys -b /sdcard -w /root"

if [ ! -x "$ROOTFS/bin/sh" ]; then
  if command -v proot-distro >/dev/null 2>&1; then
    say "installing Debian $RC_ARCH with proot-distro"
    proot-distro install debian >&2
    ROOTFS="${RC_ANDROID_ROOTFS:-$PREFIX/var/lib/proot-distro/installed-rootfs/debian}"
    PROOT_ARGS="-r $ROOTFS -b /dev -b /proc -b /sys -b /sdcard -w /root"
  else
    say "unpacking Debian $RC_ARCH rootfs into $ROOTFS"
    mkdir -p "$ROOTFS"
    ROOTFS_URL="${RC_ANDROID_ROOTFS_URL:-}"
    [ -n "$ROOTFS_URL" ] || {
      say "no rootfs found and no RC_ANDROID_ROOTFS_URL set."
      say "Either install proot-distro (pkg install proot-distro) or point"
      say "RC_ANDROID_ROOTFS_URL at a Debian $RC_ARCH rootfs tarball."
      exit 1
    }
    curl -fsSL "$ROOTFS_URL" | tar -xz -C "$ROOTFS" || {
      say "could not fetch $ROOTFS_URL"
      exit 1
    }
  fi
fi

# guest runs a command inside the rootfs. Defined after rootfs resolution so
# PROOT_ARGS reflects a rootfs installed by proot-distro above.
guest() { PROOT_LOADER="$LOADER" HOME=/root proot $PROOT_ARGS "$@"; }

# A slim rootfs ships no CA bundle, so TLS to the RC server fails.
if [ ! -s "$ROOTFS/etc/ssl/certs/ca-certificates.crt" ]; then
  mkdir -p "$ROOTFS/etc/ssl/certs"
  for bundle in "$PREFIX/etc/tls/cert.pem" /system/etc/security/cacerts/*; do
    [ -f "$bundle" ] || continue
    cat "$bundle" >> "$ROOTFS/etc/ssl/certs/ca-certificates.crt" 2>/dev/null || true
  done
fi

if [ ! -x "$ROOTFS/root/rc" ]; then
  say "installing rc $RC_ARCH"
  BIN_URL="${RC_ANDROID_BIN_URL:-https://github.com/OhRats-Technologies/rc/releases/latest/download/rc-linux-$RC_ARCH.tar.gz}"
  curl -fsSL "$BIN_URL" | tar -xz -C "$ROOTFS/root" || {
    say "could not fetch $BIN_URL"
    exit 1
  }
fi

if [ -s "$ROOTFS/root/.config/rc/device.json" ]; then
  say "enrollment: unchanged (this rootfs already has a device identity)"
else
  ENROLL_ARGS="$TOKEN --url $SERVER"
  [ -n "$NAME" ] && ENROLL_ARGS="$ENROLL_ARGS --name $NAME"
  # shellcheck disable=SC2086
  guest /root/rc enroll $ENROLL_ARGS
fi

say "starting the RC node"
guest /root/rc status
