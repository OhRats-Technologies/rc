#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/crypto-ed25519 >/dev/null
  scripts/build-component.sh components/key-custody-fixture >/dev/null
  scripts/build-component.sh components/key-custody-local >/dev/null
fi
for artifact in crypto-ed25519 key-custody-fixture key-custody-local; do
  test -f "dist/components/$artifact.wasm" || {
    echo "missing $artifact component artifact" >&2
    exit 1
  }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
cp dist/components/crypto-ed25519.wasm "$components/crypto-ed25519.wasm"
cp dist/components/key-custody-fixture.wasm "$components/key-custody-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() { "$kernel" --component-dir "$components" "$@"; }

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:crypto-ed25519" "$directory/waiting.out" | grep -F Active >/dev/null
grep -F "ohrats:key-custody-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null

cp dist/components/key-custody-local.wasm "$components/key-custody-local.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:key-custody-local" "$directory/active.out" | grep -F Active >/dev/null
grep -F "ohrats:key-custody-fixture" "$directory/active.out" | grep -F Active >/dev/null

slot="node:fixture/identity"
public=$(run key-custody-public "$slot")
printf '%s' "$public" | grep -E '^[0-9a-f]{64}$' >/dev/null
run key-custody-verify "$slot" | grep -Fx "key custody fixture: ok" >/dev/null
test "$(run key-custody-public "$slot")" = "$public"

key_file=$(find "$directory/state/keys" -type f -name '*.ed25519' -print)
test -n "$key_file"
test "$(printf '%s\n' "$key_file" | wc -l | tr -d ' ')" = 1
if stat -f '%Lp' "$key_file" >/dev/null 2>&1; then
  mode=$(stat -f '%Lp' "$key_file")
else
  mode=$(stat -c '%a' "$key_file")
fi
test "$mode" = 600

rm "$components/key-custody-local.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F "ohrats:key-custody-fixture" "$directory/removed.out" | grep -F Waiting >/dev/null
cp dist/components/key-custody-local.wasm "$components/key-custody-local.wasm"
test "$(run key-custody-public "$slot")" = "$public"

run key-custody-remove "$slot" | grep -Fx "removed" >/dev/null
run key-custody-lookup "$slot" | grep -Fx "missing" >/dev/null
test "$(find "$directory/state/keys" -type f -name '*.ed25519' | wc -l | tr -d ' ')" = 0

echo "key custody smoke: ok"
