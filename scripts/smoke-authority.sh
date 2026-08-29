#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/authority-fixture >/dev/null
  scripts/build-component.sh components/authority-store >/dev/null
fi
for artifact in authority-fixture authority-store; do
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
cp dist/components/authority-fixture.wasm "$components/authority-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() { "$kernel" --component-dir "$components" "$@"; }

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:authority-fixture" "$directory/waiting.out" | grep -F Active >/dev/null
cp dist/components/authority-store.wasm "$components/authority-store.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:authority-store" "$directory/active.out" | grep -F Active >/dev/null

fixture="vector"
seed_output=$(run authority-seed "$fixture")
test "${seed_output%% *}" = 0
hash=${seed_output#* }
printf '%s' "$hash" | grep -E '^[0-9a-f]{64}$' >/dev/null
expected=$(python3 -c 'import json; print(json.load(open("fixtures/authority-snapshot.json"))["canonicalSha256"])')
test "$hash" = "$expected"

# A new kernel process restores the same TOFU snapshot and does not emit a
# transition invalidation for generation zero.
run authority-verify "$fixture" | grep -Fx "authority state: ok" >/dev/null

python3 - "$directory/kernel.sqlite3" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
rows = connection.execute(
    "SELECT key,value FROM rc_component_entries WHERE owner=?",
    ("ohrats:authority-store",),
).fetchall()
assert rows
assert all(b"private" not in key.lower() + value.lower() for key, value in rows)
assert all(b"signature" not in key.lower() + value.lower() for key, value in rows)
PY

echo "authority storage smoke: ok"
