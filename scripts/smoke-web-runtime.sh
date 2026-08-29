#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

build="webui-shell diagnostics-store diagnostics-ui"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in $build; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi
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
cp dist/components/webui-shell.wasm "$components/webui-shell.wasm"
cp dist/components/diagnostics-store.wasm "$components/diagnostics-store.wasm"

log="$directory/kernel.log"
kernel/target/debug/rc-kernel \
  --component-dir "$components" \
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

wait_code() {
  path=$1
  expected=$2
  output=$3
  count=0
  while :; do
    code=$(curl -sS -o "$output" -w '%{http_code}' "$base$path" || true)
    [ "$code" = "$expected" ] && return 0
    count=$((count + 1))
    [ "$count" -lt 120 ] || {
      echo "$path returned $code, expected $expected" >&2
      cat "$log" >&2
      return 1
    }
    sleep 0.05
  done
}

wait_code /healthz 200 "$directory/health"
grep -Fx ok "$directory/health" >/dev/null
wait_code / 200 "$directory/home.html"
grep -F 'Remote Control<br/><span class="hero-muted">for your machines.</span>' "$directory/home.html" >/dev/null
wait_code /login 404 "$directory/login.html"
wait_code /setup 404 "$directory/setup.html"
wait_code /docs 200 "$directory/docs.html"
grep -F 'class="docs-sidebar"' "$directory/docs.html" >/dev/null
grep -F 'class="docs-toc"' "$directory/docs.html" >/dev/null
grep -F '<h1>Quickstart</h1>' "$directory/docs.html" >/dev/null
wait_code /docs/principles 200 "$directory/principles.html"
grep -F '<h1>Principles</h1>' "$directory/principles.html" >/dev/null
wait_code /docs/security 200 "$directory/security.html"
grep -F '<h1>Security model</h1>' "$directory/security.html" >/dev/null
wait_code /docs/authentication 200 "$directory/authentication.html"
grep -F '<h1>Authentication</h1>' "$directory/authentication.html" >/dev/null
wait_code /docs/cli 200 "$directory/cli.html"
grep -F 'rc --help' "$directory/cli.html" >/dev/null
wait_code /docs/mcp 200 "$directory/mcp.html"
wait_code /docs/api 200 "$directory/api.html"
wait_code /not-real 404 "$directory/missing"
grep -Fx 'not found' "$directory/missing" >/dev/null
wait_code /diagnostics 404 "$directory/no-diagnostics"

grep -o '/assets/rc\.[0-9a-f]*\.css' "$directory/home.html" | sort -u |
while IFS= read -r css; do
  curl -fsS -D "$directory/css.headers" "$base$css" -o "$directory/$(basename "$css")"
  grep -Fi 'cache-control: public, max-age=31536000, immutable' "$directory/css.headers" >/dev/null
  curl -fsS -I "$base$css" | grep -Fi 'content-type: text/css' >/dev/null
done
script=$(grep -o '/assets/rc\.[0-9a-f]*\.js' "$directory/docs.html" | head -1)
test -n "$script"
curl -fsS "$base$script" | grep -F 'navigator.clipboard.writeText' >/dev/null
card=$(grep -o '/assets/rc\.[0-9a-f]*\.png' "$directory/home.html" | head -1)
test -n "$card"
curl -fsS -I "$base$card" | grep -Fi 'content-type: image/png' >/dev/null

# Route contributions appear and disappear while the same native listener runs.
temporary="$components/diagnostics-ui.wasm.new"
cp dist/components/diagnostics-ui.wasm "$temporary"
mv "$temporary" "$components/diagnostics-ui.wasm"
wait_code /diagnostics 200 "$directory/diagnostics.html"
grep -F '<h1>Diagnostics</h1>' "$directory/diagnostics.html" >/dev/null
rm "$components/diagnostics-store.wasm"
wait_code /diagnostics 404 "$directory/diagnostics-without-store"
cp dist/components/diagnostics-store.wasm "$components/diagnostics-store.wasm"
wait_code /diagnostics 200 "$directory/diagnostics-restored.html"

# The native health endpoint stays up even if the WebUI provider is withdrawn.
rm "$components/webui-shell.wasm"
wait_code / 404 "$directory/without-webui"
wait_code /healthz 200 "$directory/health-without-webui"
cp dist/components/webui-shell.wasm "$components/webui-shell.wasm"
wait_code / 200 "$directory/home-restored.html"

echo "web runtime smoke: ok"
