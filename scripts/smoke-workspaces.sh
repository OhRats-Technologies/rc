#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/workspace-store >/dev/null
  scripts/build-component.sh components/workspace-fixture >/dev/null
fi
for artifact in workspace-store workspace-fixture; do test -f "dist/components/$artifact.wasm" || { echo "missing $artifact component artifact" >&2; exit 1; }; done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"; mkdir -p "$components"
cp dist/components/workspace-fixture.wasm "$components/workspace-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() { "$kernel" --component-dir "$components" "$@"; }
run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:workspace-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null
if run workspace-seed unavailable >/dev/null 2>&1; then echo "workspace fixture activated without provider" >&2; exit 1; fi
cp dist/components/workspace-store.wasm "$components/workspace-store.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:workspace-store" "$directory/active.out" | grep -F Active >/dev/null
grep -F "ohrats:workspace-fixture" "$directory/active.out" | grep -F Active >/dev/null
fixture="smoke-$(date +%s)-$$"
payload=$(run workspace-seed "$fixture" 2>"$directory/seed.err")
token=${payload#*:}; test "$token" != "$payload"
# A fresh kernel process must restore the durable state.
run workspace-verify "$fixture" "$payload" >"$directory/verify.out" 2>"$directory/verify.err"
grep -Fx "workspace state: ok" "$directory/verify.out" >/dev/null
python3 - "$directory/kernel.sqlite3" "$token" <<'PY'
import sqlite3, sys
db, token = sys.argv[1], sys.argv[2].encode()
connection = sqlite3.connect(db)
rows = connection.execute("SELECT key,value FROM rc_component_entries WHERE owner=?", ("ohrats:workspace-store",)).fetchall()
assert rows
assert all(token not in key and token not in value for key, value in rows)
PY
rm "$components/workspace-store.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F "ohrats:workspace-fixture" "$directory/removed.out" | grep -F Waiting >/dev/null
echo "workspace storage smoke: ok"
