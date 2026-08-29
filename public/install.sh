#!/bin/sh
set -eu
# The release bundle is the POSIX bootstrap boundary.
TOKEN="${1:-${RC_ENROLL_TOKEN:-}}"
SERVER="${2:-${RC_URL:-}}"
HOME_DIR="${HOME:-}"
API="${RC_RELEASE_API:-https://api.github.com/repos/OhRats-Technologies/rc/releases/latest}"
BIN_DIR="${RC_INSTALL_BIN_DIR:-$HOME_DIR/.local/bin}"
DATA_DIR="${RC_DATA_DIR:-$HOME_DIR/.local/share/rc}"
COMPONENT_DIR="${RC_COMPONENT_DIR:-$DATA_DIR/components}"
ROLLBACK_DIR="${RC_INSTALL_ROLLBACK_DIR:-$DATA_DIR/rollback}"
STATE_DIR="${RC_STATE_DIR:-$HOME_DIR/.config/rc}"
test -n "$HOME_DIR" || { echo "HOME is required" >&2; exit 1; }
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
case "$API" in
  http://*|https://*) ;;
  *) echo "release API URL must use http or https" >&2; exit 1 ;;
esac
TMPDIR_PATH=$(mktemp -d "${TMPDIR:-/tmp}/rc-install.XXXXXX")
trap 'rm -rf "$TMPDIR_PATH"' EXIT HUP INT TERM
JSON="$TMPDIR_PATH/release.json"

download() {
  url=$1
  destination=$2
  limit=$3
  curl -fsSL --max-filesize "$limit" "$url" -o "$destination"
  test "$(wc -c < "$destination")" -le "$limit" ||
    { echo "download exceeds installer limit: $url" >&2; exit 1; }
}
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else echo "sha256sum or shasum is required" >&2; exit 1
  fi
}
asset_object() {
  asset_name=$1
  tr -d '\n\r\t ' < "$JSON" |
    awk -v wanted="$asset_name" 'BEGIN { RS="\\},\\{" }
      index($0, "\"name\":\"" wanted "\"") { print; exit }'
}
asset_field() {
  asset_name=$1
  field=$2
  asset_object "$asset_name" |
    sed -n "s/.*\"$field\":\"\([^\"]*\)\".*/\1/p"
}
verify_asset() {
  asset_name=$1
  asset_limit=$2
  asset_url=$(asset_field "$asset_name" browser_download_url)
  asset_digest=$(asset_field "$asset_name" digest)
  case "$asset_url" in
    https://github.com/OhRats-Technologies/rc/releases/download/*) ;;
    *) echo "release asset has an invalid download URL: $asset_name" >&2; exit 1 ;;
  esac
  case "$asset_digest" in
    sha256:????????????????????????????????????????????????????????????????) ;;
    *) echo "release asset is missing a SHA-256 digest: $asset_name" >&2; exit 1 ;;
  esac
  expected=$(printf '%s' "${asset_digest#sha256:}" | tr '[:upper:]' '[:lower:]')
  case "$expected" in
    *[!0-9a-f]*) echo "release asset has an invalid SHA-256 digest: $asset_name" >&2; exit 1 ;;
  esac
  destination="$TMPDIR_PATH/$asset_name"
  download "$asset_url" "$destination" "$asset_limit"
  actual=$(sha256_file "$destination" | tr '[:upper:]' '[:lower:]')
  test "$actual" = "$expected" ||
    { echo "downloaded asset checksum mismatch: $asset_name" >&2; exit 1; }
}
download "$API" "$JSON" $((4 << 20))

validate_single_archive() {
  archive=$1
  member=$2
  listing=$TMPDIR_PATH/"$member.list"
  tar -tzf "$archive" > "$listing"
  tar -tvzf "$archive" | awk '$1 !~ /^-/ { exit 1 }' ||
    { echo "archive contains a non-regular member" >&2; exit 1; }
  test "$(wc -l < "$listing")" -eq 1 && test "$(sed -n '1p' "$listing")" = "$member" ||
    { echo "archive must contain only $member" >&2; exit 1; }
}

validate_bundle_archive() {
  archive=$1
  listing=$TMPDIR_PATH/core.list
  tar -tzf "$archive" > "$listing"
  tar -tvzf "$archive" | awk '$1 !~ /^[-d]/ { exit 1 }' ||
    { echo "core bundle contains a non-file member" >&2; exit 1; }
  awk '
    function allowed_component(name) {
      return name == "artifact-cache-local" || name == "diagnostics-cli" || name == "diagnostics-reporter" ||
        name == "diagnostics-store" || name == "github-source" ||
        name == "http-source" || name == "local-source" ||
        name == "oci-source" || name == "package-manager" ||
        name == "process-policy" || name == "transport-webrtc" ||
        name == "updater"
    }
    {
      if (seen[$0]++) exit 1
      if ($0 == "components" || $0 == "components/") next
      if ($0 == "profile.lock") { locks++; next }
      if (index($0, "components/") == 1 && $0 ~ /\.wasm$/) {
        name = substr($0, 12); sub(/\.wasm$/, "", name)
        if (!allowed_component(name)) exit 1
        components++
        next
      }
      exit 1
    }
    END { if (locks != 1 || components != 12) exit 1 }
  ' "$listing" || { echo "invalid core bundle members" >&2; exit 1; }
}

validate_lock() {
  lock=$1
  awk '
    NR == 1 && $0 == "schema 1" { next }
    NR == 2 && $0 == "profile ohrats:core" { next }
    $1 == "component" && NF == 3 {
      digest=$3
      if (seen[$2]++ || length(digest) != 71 || substr(digest, 1, 7) != "sha256:") exit 1
      for (i = 8; i <= length(digest); i++)
        if (tolower(substr(digest, i, 1)) !~ /[0-9a-f]/) exit 1
      count++
      next
    }
    { exit 1 }
    END { if (count != 12) exit 1 }
  ' "$lock" || { echo "invalid core profile lock" >&2; exit 1; }
}

CORE_COMPONENTS="artifact-cache-local diagnostics-cli diagnostics-reporter diagnostics-store github-source http-source local-source oci-source package-manager process-policy transport-webrtc updater"
component_digest() {
  awk -v wanted="$1" '$1 == "component" && $2 == wanted { print $3; exit }' "$TMPDIR_PATH/profile.lock"
}

TAG=$(tr -d '\n\r\t ' < "$JSON" | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p')
VERSION=${TAG#v}
case "$VERSION" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "release has an invalid semantic version" >&2; exit 1 ;;
esac

PLATFORM="${OS}-${ARCH}"
verify_asset "rc-${PLATFORM}.tar.gz" $((160 << 20))
verify_asset "rc-kernel-${PLATFORM}.tar.gz" $((160 << 20))
verify_asset "rc-core-components.tar.gz" $((128 << 20))

validate_single_archive "$TMPDIR_PATH/rc-${PLATFORM}.tar.gz" rc
validate_single_archive "$TMPDIR_PATH/rc-kernel-${PLATFORM}.tar.gz" rc-kernel
validate_bundle_archive "$TMPDIR_PATH/rc-core-components.tar.gz"

mkdir -p "$TMPDIR_PATH/new/components"
tar -xzf "$TMPDIR_PATH/rc-${PLATFORM}.tar.gz" -C "$TMPDIR_PATH/new"
tar -xzf "$TMPDIR_PATH/rc-kernel-${PLATFORM}.tar.gz" -C "$TMPDIR_PATH/new"
tar -xzf "$TMPDIR_PATH/rc-core-components.tar.gz" -C "$TMPDIR_PATH/new"
mv "$TMPDIR_PATH/new/profile.lock" "$TMPDIR_PATH/profile.lock"
validate_lock "$TMPDIR_PATH/profile.lock"

for name in $CORE_COMPONENTS; do
  file="$TMPDIR_PATH/new/components/$name.wasm"
  test -f "$file" || { echo "core bundle is missing $name" >&2; exit 1; }
  expected=$(component_digest "$name")
  actual="sha256:$(sha256_file "$file")"
  test "$actual" = "$expected" ||
    { echo "core component digest mismatch: $name" >&2; exit 1; }
done

chmod 0755 "$TMPDIR_PATH/new/rc" "$TMPDIR_PATH/new/rc-kernel"
test "$("$TMPDIR_PATH/new/rc" version)" = "RC $VERSION" ||
  { echo "downloaded rc did not report the release version" >&2; exit 1; }
"$TMPDIR_PATH/new/rc-kernel" --version | grep -Eq '^RC kernel [0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$' ||
  { echo "downloaded kernel did not report a valid version" >&2; exit 1; }
"$TMPDIR_PATH/new/rc-kernel" --component-dir "$TMPDIR_PATH/new/components" repair >/dev/null ||
  { echo "core profile failed kernel validation" >&2; exit 1; }

mkdir -p "$BIN_DIR" "$COMPONENT_DIR" "$ROLLBACK_DIR"
BACKUP="$ROLLBACK_DIR/previous"
rm -rf "$BACKUP.new"
mkdir -p "$BACKUP.new/components"
for path in rc rc-kernel; do
  if [ -f "$BIN_DIR/$path" ]; then cp -p "$BIN_DIR/$path" "$BACKUP.new/$path"; fi
done
for name in $CORE_COMPONENTS; do
  for suffix in wasm core; do
    if [ -f "$COMPONENT_DIR/$name.$suffix" ]; then
      cp -p "$COMPONENT_DIR/$name.$suffix" "$BACKUP.new/components/$name.$suffix"
    fi
  done
done
if [ -f "$BIN_DIR/rc" ] || [ -f "$BIN_DIR/rc-kernel" ]; then
  old_version=$("$BIN_DIR/rc" version 2>/dev/null || :)
  printf '%s\n' "$old_version" > "$BACKUP.new/version"
fi
rm -rf "$BACKUP"
mv "$BACKUP.new" "$BACKUP"

restore_previous() {
  for path in rc rc-kernel; do
    if [ -f "$BACKUP/$path" ]; then
      cp "$BACKUP/$path" "$BIN_DIR/$path.rc-restore.$$"
      chmod 0755 "$BIN_DIR/$path.rc-restore.$$"
      mv "$BIN_DIR/$path.rc-restore.$$" "$BIN_DIR/$path"
    else
      rm -f "$BIN_DIR/$path"
    fi
  done
  for name in $CORE_COMPONENTS; do
    for suffix in wasm core; do
      old="$BACKUP/components/$name.$suffix"
      current="$COMPONENT_DIR/$name.$suffix"
      if [ -f "$old" ]; then
        cp "$old" "$current.rc-restore.$$"
        mv "$current.rc-restore.$$" "$current"
      else
        rm -f "$current"
      fi
    done
  done
}

cleanup() {
  status=$?
  if [ "${ACTIVATING:-0}" -eq 1 ] && [ "$status" -ne 0 ]; then
    restore_previous
  fi
  rm -rf "$TMPDIR_PATH"
  trap - EXIT HUP INT TERM
  exit "$status"
}
ACTIVATING=0
trap cleanup EXIT HUP INT TERM

install_file() {
  source=$1
  destination=$2
  mode=$3
  temporary="$destination.rc-install.$$"
  cp "$source" "$temporary"
  chmod "$mode" "$temporary"
  mv "$temporary" "$destination"
}

ACTIVATING=1
install_file "$TMPDIR_PATH/new/rc" "$BIN_DIR/rc" 0755
install_file "$TMPDIR_PATH/new/rc-kernel" "$BIN_DIR/rc-kernel" 0755
for name in $CORE_COMPONENTS; do
  target="$COMPONENT_DIR/$name.wasm"
  marker="$COMPONENT_DIR/$name.core"
  if [ -f "$target" ] && {
    [ ! -f "$marker" ] || [ "$(cat "$marker")" != "sha256:$(sha256_file "$target")" ];
  }; then
    echo "preserving locally overridden component $target" >&2
    continue
  fi
  install_file "$TMPDIR_PATH/new/components/$name.wasm" "$target" 0644
  printf 'sha256:%s\n' "$(sha256_file "$TMPDIR_PATH/new/components/$name.wasm")" > "$marker.rc-install.$$"
  mv "$marker.rc-install.$$" "$marker"
done
printf '%s\n' "$VERSION" > "$ROLLBACK_DIR/installed-version.tmp"
mv "$ROLLBACK_DIR/installed-version.tmp" "$ROLLBACK_DIR/installed-version"
ACTIVATING=0

echo "installed RC $VERSION in $BIN_DIR"
if [ -n "$TOKEN" ]; then
  if [ -n "$SERVER" ]; then
    "$BIN_DIR/rc" enroll "$TOKEN" --url "$SERVER"
  else
    "$BIN_DIR/rc" enroll "$TOKEN"
  fi
fi
if [ -s "$STATE_DIR/device.json" ]; then
  if "$BIN_DIR/rc" service install; then
    echo "node:   running in the background"
  else
    echo "warning: could not start the background service" >&2
    echo "run:    rc run" >&2
  fi
else
  echo "enroll: rc enroll ENROLLMENT_TOKEN"
fi
echo "help:   rc --help"
