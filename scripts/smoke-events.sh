#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/events-store >/dev/null
  scripts/build-component.sh components/events-fixture >/dev/null
fi
for artifact in events-store events-fixture; do
  test -f "dist/components/$artifact.wasm" || { echo "missing $artifact component artifact" >&2; exit 1; }
done
if [ "${RC_SKIP_KERNEL_BUILD:-0}" != 1 ]; then
  cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
fi

directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
cp dist/components/events-fixture.wasm "$components/events-fixture.wasm"
kernel=${RC_KERNEL_BIN:-kernel/target/debug/rc-kernel}
test -x "$kernel" || { echo "missing RC kernel binary" >&2; exit 1; }
run() { "$kernel" --component-dir "$components" "$@"; }

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:events-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null
cp dist/components/events-store.wasm "$components/events-store.wasm"
fixture="smoke-$(date +%s)-$$"
run events-seed "$fixture" >"$directory/seed.out" 2>"$directory/seed.err"

# A fresh kernel process must recover the component-owned cursor and records.
run events-verify "$fixture" >"$directory/verify.out" 2>"$directory/verify.err"
grep -Fx "events state: ok" "$directory/verify.out" >/dev/null

# The typed schema cannot hold command or output fields; also reject accidental plaintext.
python3 - "$directory/kernel.sqlite3" <<'PY'
import sqlite3
import sys
import json

connection = sqlite3.connect(sys.argv[1])
rows = connection.execute(
    "SELECT value FROM rc_component_entries WHERE owner=? AND bucket='lifecycle-events'",
    ("ohrats:events-store",),
).fetchall()
assert len(rows) == 2
for (value,) in rows:
    assert b'command' not in value and b'stdout' not in value and b'stderr' not in value
    assert b'transcript' not in value and b'plaintext-secret' not in value

idempotency_rows = connection.execute(
    "SELECT value FROM rc_component_entries WHERE owner=? AND bucket='event-idempotency'",
    ("ohrats:events-store",),
).fetchall()
assert len(idempotency_rows) == 1
event_cursors = {json.loads(value)["cursor"] for (value,) in rows}
assert json.loads(idempotency_rows[0][0])["cursor"] in event_cursors
assert len(idempotency_rows) <= len(rows)
PY

echo "events storage smoke: ok"
