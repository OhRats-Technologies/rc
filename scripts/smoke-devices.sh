#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/device-store >/dev/null
  scripts/build-component.sh components/device-fixture >/dev/null
fi
for artifact in device-store device-fixture; do
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
cp dist/components/device-fixture.wasm "$components/device-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:device-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null
cp dist/components/device-store.wasm "$components/device-store.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:device-store" "$directory/active.out" | grep -F Active >/dev/null
grep -F "ohrats:device-fixture" "$directory/active.out" | grep -F Active >/dev/null

fixture="smoke-$(date +%s)-$$"
seed_output=$(run devices-seed "$fixture" 2>"$directory/seed.err")
token=${seed_output%% *}
device=${seed_output#* }
test "$token $device" = "$seed_output"
printf '%s' "$token" | grep -E '^enroll_[A-Za-z0-9_-]{43}$' >/dev/null
printf '%s' "$device" | grep -E '^[0-9a-f-]{36}$' >/dev/null

python3 - "$directory/kernel.sqlite3" "$token" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
token = sys.argv[2].encode()
rows = connection.execute(
    "SELECT bucket,key,value FROM rc_component_entries WHERE owner=?",
    ("ohrats:device-store",),
).fetchall()
assert rows
assert all(token not in key and token not in value for _, key, value in rows)
token_rows = [row for row in rows if row[0] == "enrollment-tokens"]
assert token_rows and all(len(row[1]) == 32 for row in token_rows)
assert all(b"private" not in value.lower() and b"signature" not in value.lower() for _, _, value in rows)
PY

# A new kernel process must recover registry, presence, and enrollment state.
run devices-verify "$fixture" "$device" >"$directory/verify.out" 2>"$directory/verify.err"
grep -Fx "device state: ok" "$directory/verify.out" >/dev/null

# Tombstones and limits survive another restart.
if run devices-verify "$fixture" "$device" >/dev/null 2>&1; then
  echo "revoked device was restored as active" >&2
  exit 1
fi

rm "$components/device-store.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F "ohrats:device-fixture" "$directory/removed.out" | grep -F Waiting >/dev/null

echo "device storage smoke: ok"
