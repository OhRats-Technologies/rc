#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/identity-store >/dev/null
  scripts/build-component.sh components/identity-fixture >/dev/null
fi
for artifact in identity-store identity-fixture; do
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
cp dist/components/identity-fixture.wasm "$components/identity-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:identity-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null
if run identity-seed unavailable >/dev/null 2>&1; then
  echo "identity fixture activated without its provider" >&2
  exit 1
fi

cp dist/components/identity-store.wasm "$components/identity-store.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:identity-store" "$directory/active.out" | grep -F Active >/dev/null
grep -F "ohrats:identity-fixture" "$directory/active.out" | grep -F Active >/dev/null

fixture="smoke-$(date +%s)-$$"
token=$(run identity-seed "$fixture" 2>"$directory/seed.err")
printf '%s' "$token" | grep -E '^[A-Za-z0-9_-]{43}$' >/dev/null

# A second kernel process must recover the component-owned identity records.
run identity-verify "$fixture" "$token" >"$directory/verify.out" 2>"$directory/verify.err"
grep -Fx "identity state: ok" "$directory/verify.out" >/dev/null
if run identity-verify "$fixture" "$token" >/dev/null 2>&1; then
  echo "consumed identity fixture state was reusable" >&2
  exit 1
fi

# Raw bearer material is never written to the component database.
python3 - "$directory/kernel.sqlite3" "$token" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
token = sys.argv[2].encode()
rows = connection.execute(
    "SELECT key,value FROM rc_component_entries WHERE owner=?",
    ("ohrats:identity-store",),
).fetchall()
assert rows
assert all(token not in key and token not in value for key, value in rows)
assert connection.execute(
    "SELECT count(*) FROM rc_component_entries WHERE owner=? AND bucket='browser-sessions'",
    ("ohrats:identity-store",),
).fetchone() == (0,)
assert connection.execute(
    "SELECT count(*) FROM rc_component_entries WHERE owner=? AND bucket='ceremonies'",
    ("ohrats:identity-store",),
).fetchone() == (0,)
PY

rm "$components/identity-store.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F "ohrats:identity-fixture" "$directory/removed.out" | grep -F Waiting >/dev/null

echo "identity storage smoke: ok"
