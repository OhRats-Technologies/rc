#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in identity-store identity-fixture webauthn-es256 webui-shell webui-app; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi
cargo test --manifest-path components/webui-shell/Cargo.toml --locked \
  contribution -- --nocapture >/dev/null
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
kernel_pid=
cleanup() {
  if [ -n "$kernel_pid" ]; then
    kill "$kernel_pid" 2>/dev/null || true
    wait "$kernel_pid" 2>/dev/null || true
  fi
  rm -rf "$directory"
}
trap cleanup EXIT INT TERM

components="$directory/components"
mkdir -p "$components"
for component in identity-store identity-fixture webauthn-es256 webui-shell webui-app; do
  cp "dist/components/$component.wasm" "$components/$component.wasm"
done

kernel=kernel/target/debug/rc-kernel
fixture="authenticated-webui-$$"
token=$(
  "$kernel" --component-dir "$components" identity-seed "$fixture" \
    2>"$directory/seed.err"
)
printf '%s' "$token" | grep -E '^[A-Za-z0-9_-]{43}$' >/dev/null

log="$directory/kernel.log"
"$kernel" --component-dir "$components" \
  serve --listen 127.0.0.1:0 >"$log" 2>&1 &
kernel_pid=$!
count=0
while ! grep -E 'RC kernel HTTP listening on 127\.0\.0\.1:[0-9]+' "$log" >/dev/null 2>&1; do
  count=$((count + 1))
  [ "$count" -lt 200 ] || { cat "$log" >&2; exit 1; }
  sleep 0.05
done
port=$(sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$log" | head -1)
base="http://127.0.0.1:$port"

curl -sS -D "$directory/signed-out.headers" -o /dev/null "$base/devices"
grep -E '^HTTP/.* 303' "$directory/signed-out.headers" >/dev/null
grep -Fi 'location: /login?next=/devices' "$directory/signed-out.headers" >/dev/null

cookie="rc_session=$token"
curl -fsS -H "Cookie: $cookie" "$base/devices" >"$directory/devices.html"
grep -F '<h1>Devices</h1>' "$directory/devices.html" >/dev/null
grep -F 'Identity Fixture' "$directory/devices.html" >/dev/null
grep -F 'data-sidebar="open"' "$directory/devices.html" >/dev/null
grep -F 'data-navigation-id="devices"' "$directory/devices.html" >/dev/null

curl -fsS -H "Cookie: $cookie; rc_sidebar=closed" \
  "$base/account" >"$directory/account.html"
grep -F '<h1>Identity Fixture</h1>' "$directory/account.html" >/dev/null
grep -F 'data-sidebar="closed"' "$directory/account.html" >/dev/null

# Public pages remain component-owned historical snapshots beside authenticated routes.
curl -fsS "$base/" >"$directory/landing.html"
curl -fsS "$base/docs" >"$directory/docs.html"
grep -F 'Remote Control<br/><span class="hero-muted">for your machines.</span>' \
  "$directory/landing.html" >/dev/null
grep -F '<h1>Quickstart</h1>' "$directory/docs.html" >/dev/null
grep -F 'logo.092a1cece4d0.svg' "$directory/landing.html" >/dev/null
grep -F 'ohrats.eb38b77e6b5e.css' "$directory/docs.html" >/dev/null

# Withdrawal removes authenticated routes while leaving the public shell healthy.
rm "$components/webui-app.wasm"
count=0
while :; do
  code=$(curl -sS -o /dev/null -w '%{http_code}' -H "Cookie: $cookie" "$base/devices")
  [ "$code" = 404 ] && break
  count=$((count + 1))
  [ "$count" -lt 120 ] || { cat "$log" >&2; exit 1; }
  sleep 0.05
done
curl -fsS "$base/" >/dev/null

echo "authenticated WebUI smoke: ok"
