#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/rc-install-smoke.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
mkdir -p "$fixture/bin" "$fixture/components" "$fixture/home/.local/bin" \
  "$fixture/home/.local/share/rc/components"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

cat > "$fixture/rc" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version) echo 'RC 1.0.0' ;;
  enroll) exit 0 ;;
  service) printf 'service-called\n' > "$RC_INSTALL_FIXTURE/service-called" ;;
  *) exit 0 ;;
esac
EOF
cat > "$fixture/rc-kernel" <<'EOF'
#!/bin/sh
case "${1:-}" in
  --version) echo 'RC kernel 1.0.0' ;;
  --component-dir) exit 0 ;;
  *) exit 0 ;;
esac
EOF
chmod 0755 "$fixture/rc" "$fixture/rc-kernel"
tar -C "$fixture" -czf "$fixture/rc-linux-amd64.tar.gz" rc
tar -C "$fixture" -czf "$fixture/rc-kernel-linux-amd64.tar.gz" rc-kernel

for name in artifact-cache-local diagnostics-cli diagnostics-reporter diagnostics-store execution-runtime github-source \
  http-source local-source oci-source package-manager process-policy scheduler shell transport-webrtc updater; do
  printf 'fixture:%s\n' "$name" > "$fixture/components/$name.wasm"
done
"$root/packaging/build-core-bundle.sh" "$fixture/rc-core-profile.tar.gz" \
  "$fixture/components" >/dev/null
"$root/packaging/build-legacy-core-bundle.sh" "$fixture/rc-core-components.tar.gz" \
  "$fixture/components" >/dev/null

cat > "$fixture/release.json" <<EOF
{
  "tag_name": "v1.0.0",
  "assets": [
    {"name": "rc-linux-amd64.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-linux-amd64.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-linux-amd64.tar.gz")"},
    {"name": "rc-kernel-linux-amd64.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-kernel-linux-amd64.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-kernel-linux-amd64.tar.gz")"},
    {"name": "rc-core-components.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-core-components.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-core-components.tar.gz")"},
    {"name": "rc-core-profile.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-core-profile.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-core-profile.tar.gz")"}
  ]
}
EOF
cat > "$fixture/release-legacy.json" <<EOF
{
  "tag_name": "v1.0.0",
  "assets": [
    {"name": "rc-linux-amd64.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-linux-amd64.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-linux-amd64.tar.gz")"},
    {"name": "rc-kernel-linux-amd64.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-kernel-linux-amd64.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-kernel-linux-amd64.tar.gz")"},
    {"name": "rc-core-components.tar.gz", "browser_download_url": "https://github.com/OhRats-Technologies/rc/releases/download/v1.0.0/rc-core-components.tar.gz", "digest": "sha256:$(hash_file "$fixture/rc-core-components.tar.gz")"}
  ]
}
EOF

cat > "$fixture/bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 1 ;;
esac
EOF
cat > "$fixture/bin/curl" <<'EOF'
#!/bin/sh
output=
url=
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    output=$1
  else
    url=$1
  fi
  shift
done
case "$url" in
  https://api.github.com/*) source="$RC_INSTALL_FIXTURE/${RC_INSTALL_RELEASE_JSON:-release.json}" ;;
  *rc-linux-amd64.tar.gz) source="$RC_INSTALL_FIXTURE/rc-linux-amd64.tar.gz" ;;
  *rc-kernel-linux-amd64.tar.gz) source="$RC_INSTALL_FIXTURE/rc-kernel-linux-amd64.tar.gz" ;;
  *rc-core-profile.tar.gz) source="$RC_INSTALL_FIXTURE/rc-core-profile.tar.gz" ;;
  *rc-core-components.tar.gz) source="$RC_INSTALL_FIXTURE/rc-core-components.tar.gz" ;;
  *) echo "unexpected fixture URL: $url" >&2; exit 1 ;;
esac
cp "$source" "$output"
EOF
chmod 0755 "$fixture/bin/uname" "$fixture/bin/curl"

old_rc="$fixture/home/.local/bin/rc"
cat > "$old_rc" <<'EOF'
#!/bin/sh
echo 'RC 0.9.0'
EOF
chmod 0755 "$old_rc"
cp "$fixture/rc-kernel" "$fixture/home/.local/bin/rc-kernel"
for name in artifact-cache-local diagnostics-cli diagnostics-reporter diagnostics-store execution-runtime github-source \
  http-source local-source oci-source package-manager process-policy scheduler shell transport-webrtc updater; do
  printf 'old:%s\n' "$name" > "$fixture/home/.local/share/rc/components/$name.wasm"
  printf 'sha256:%s\n' "$(hash_file "$fixture/home/.local/share/rc/components/$name.wasm")" \
    > "$fixture/home/.local/share/rc/components/$name.core"
done
mkdir -p "$fixture/custom-state"
printf 'enrolled\n' > "$fixture/custom-state/device.json"

PATH="$fixture/bin:$PATH" HOME="$fixture/home" RC_INSTALL_FIXTURE="$fixture" \
  RC_RELEASE_API=https://api.github.com/repos/OhRats-Technologies/rc/releases/latest \
  RC_STATE_DIR="$fixture/custom-state" \
  RC_INSTALL_BIN_DIR="$fixture/home/.local/bin" \
  RC_DATA_DIR="$fixture/home/.local/share/rc" sh "$root/public/install.sh"

test "$(HOME="$fixture/home" "$fixture/home/.local/bin/rc" version)" = 'RC 1.0.0'
test -x "$fixture/home/.local/bin/rc-kernel"
test -f "$fixture/home/.local/share/rc/rollback/previous/rc"
test "$(HOME="$fixture/home" "$fixture/home/.local/share/rc/rollback/previous/rc" version)" = 'RC 0.9.0'
grep -F 'old:package-manager' \
  "$fixture/home/.local/share/rc/rollback/previous/components/package-manager.wasm" >/dev/null
test -f "$fixture/service-called"
test -f "$fixture/home/.local/share/rc/components/package-manager.core"
test ! -e "$fixture/home/.local/share/rc/components/package-manager.managed"
test -f "$fixture/home/.local/share/rc/components/artifact-cache-local.wasm"
test -f "$fixture/home/.local/share/rc/components/updater.wasm"
test -f "$fixture/home/.local/share/rc/components/scheduler.wasm"
test -f "$fixture/home/.local/share/rc/components/shell.wasm"

mkdir -p "$fixture/legacy-home/.local/bin" "$fixture/legacy-data/components"
PATH="$fixture/bin:$PATH" HOME="$fixture/legacy-home" RC_INSTALL_FIXTURE="$fixture" \
  RC_INSTALL_RELEASE_JSON=release-legacy.json \
  RC_RELEASE_API=https://api.github.com/repos/OhRats-Technologies/rc/releases/latest \
  RC_INSTALL_BIN_DIR="$fixture/legacy-home/.local/bin" RC_DATA_DIR="$fixture/legacy-data" \
  sh "$root/public/install.sh"
test "$(find "$fixture/legacy-data/components" -name '*.wasm' | wc -l | tr -d ' ')" -eq 10
test ! -e "$fixture/legacy-data/components/artifact-cache-local.wasm"
test ! -e "$fixture/legacy-data/components/updater.wasm"
echo 'installer smoke: ok'
