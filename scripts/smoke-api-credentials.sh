#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/api-credential-store >/dev/null
  scripts/build-component.sh components/api-credential-fixture >/dev/null
fi
for artifact in api-credential-store api-credential-fixture; do
  test -f "dist/components/$artifact.wasm" || { echo "missing $artifact component artifact" >&2; exit 1; }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
mkdir -p "$directory/components"
cp dist/components/api-credential-store.wasm dist/components/api-credential-fixture.wasm "$directory/components/"
kernel=kernel/target/debug/rc-kernel
run() { "$kernel" --component-dir "$directory/components" "$@"; }
fixture="smoke-$(date +%s)-$$"
run api-credentials-seed "$fixture" >"$directory/seed.out" 2>"$directory/seed.err"
grep -Fx "api credential seed: ok" "$directory/seed.out" >/dev/null
# Both callers sign the same request; exactly one may win the nonce CAS.
set +e
run api-credentials-race "$fixture" >"$directory/race-a.out" 2>&1 & race_a=$!
run api-credentials-race "$fixture" >"$directory/race-b.out" 2>&1 & race_b=$!
wait "$race_a"; status_a=$?
wait "$race_b"; status_b=$?
set -e
[ "$status_a" -ne "$status_b" ] || {
  echo "same-nonce CAS did not produce one winner" >&2
  exit 1
}
# A fresh kernel process proves durable credential, revocation, nonce, and CLI state.
run api-credentials-verify "$fixture" >"$directory/verify.out" 2>"$directory/verify.err"
grep -Fx "api credential state: ok" "$directory/verify.out" >/dev/null
python3 - "$directory/kernel.sqlite3" "$fixture" <<'PY'
import hashlib
import sqlite3
import sys

db, fixture = sys.argv[1:]
connection = sqlite3.connect(db)
rows = connection.execute(
    "SELECT key,value FROM rc_component_entries WHERE owner=?",
    ("ohrats:api-credential-store",),
).fetchall()
assert rows
for key, value in rows:
    assert b"PRIVATE KEY" not in key + value
    assert f"device-{fixture}".encode() not in key + value
    assert f"user-{fixture}".encode() not in key + value
assert connection.execute(
    "SELECT count(*) FROM rc_component_entries WHERE owner=? AND bucket=?",
    ("ohrats:api-credential-store", "api-request-nonces"),
).fetchone() == (3,)
assert hashlib.sha256(f"device-{fixture}".encode()).digest() not in [key for key, _ in rows]
PY
echo "API credential smoke: ok"
