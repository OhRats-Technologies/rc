#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/storage-fixture >/dev/null
fi
test -f dist/components/storage-fixture.wasm || {
  echo "missing storage fixture artifact" >&2
  exit 1
}
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
lock_pid=
cleanup() {
  if [ -n "$lock_pid" ]; then
    kill "$lock_pid" 2>/dev/null || true
    wait "$lock_pid" 2>/dev/null || true
  fi
  rm -rf "$directory"
}
trap cleanup EXIT INT TERM

components="$directory/components"
mkdir -p "$components"
cp dist/components/storage-fixture.wasm "$components/storage-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
database="$directory/kernel.sqlite3"
run() {
  "$kernel" --component-dir "$components" "$@"
}

run kv-set items alpha one >/dev/null 2>&1
test "$(run kv-get items alpha 2>/dev/null)" = one
run kv-set items alphabet two >/dev/null 2>&1
run kv-set items beta three >/dev/null 2>&1
run kv-list items alp >"$directory/list.out" 2>/dev/null
grep -Fx 'alpha=one' "$directory/list.out" >/dev/null
grep -Fx 'alphabet=two' "$directory/list.out" >/dev/null
if grep -F beta "$directory/list.out" >/dev/null; then
  echo "storage prefix scan returned an unrelated key" >&2
  exit 1
fi
run kv-conflict >"$directory/conflict.out" 2>/dev/null
grep -F 'conflict at revision ' "$directory/conflict.out" >/dev/null

# A competing SQLite writer delays, rather than corrupting, the component commit.
ready="$directory/lock-ready"
python3 - "$database" "$ready" <<'PY' &
import pathlib
import sqlite3
import sys
import time

connection = sqlite3.connect(sys.argv[1], timeout=5)
connection.execute("BEGIN IMMEDIATE")
pathlib.Path(sys.argv[2]).write_text("ready", encoding="utf-8")
time.sleep(0.4)
connection.rollback()
PY
lock_pid=$!
count=0
while [ ! -s "$ready" ]; do
  count=$((count + 1))
  [ "$count" -lt 100 ] || { echo "lock fixture did not start" >&2; exit 1; }
  sleep 0.02
done
run kv-set items locked survived >/dev/null 2>&1
wait "$lock_pid"
lock_pid=
test "$(run kv-get items locked 2>/dev/null)" = survived

# An abruptly terminated external transaction is absent after SQLite recovery.
python3 - "$database" <<'PY' || true
import os
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
connection.execute("BEGIN IMMEDIATE")
connection.execute(
    "INSERT INTO rc_component_entries(owner,bucket,key,value) VALUES(?,?,?,?)",
    ("ohrats:storage-fixture", "items", b"crash", b"uncommitted"),
)
os._exit(17)
PY
if run kv-get items crash >"$directory/crash.out" 2>&1; then
  echo "uncommitted crash fixture became visible" >&2
  exit 1
fi
grep -F 'key "crash" is not set' "$directory/crash.out" >/dev/null

backup="$directory/backup.sqlite3"
run backup "$backup" >"$directory/backup.out" 2>/dev/null
grep -F "backup written to $backup" "$directory/backup.out" >/dev/null
python3 - "$backup" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
assert connection.execute("PRAGMA quick_check").fetchone() == ("ok",)
value = connection.execute(
    "SELECT value FROM rc_component_entries WHERE owner=? AND bucket=? AND key=?",
    ("ohrats:storage-fixture", "items", b"locked"),
).fetchone()
assert value == (b"survived",)
PY

run kv-delete items alpha >/dev/null 2>&1
if run kv-get items alpha >/dev/null 2>&1; then
  echo "deleted storage value is still visible" >&2
  exit 1
fi

echo "durable storage smoke: ok"
