#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/ssh-policy-store >/dev/null
  scripts/build-component.sh components/ssh-policy-fixture >/dev/null
fi
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
mkdir -p "$directory/components"
ssh-keygen -q -t ed25519 -N '' -f "$directory/ed25519"
ssh-keygen -q -t rsa -b 2048 -N '' -f "$directory/rsa"
ed=$(cat "$directory/ed25519.pub")
rsa=$(cat "$directory/rsa.pub")
cp dist/components/ssh-policy-fixture.wasm "$directory/components/"
cp dist/components/ssh-policy-store.wasm "$directory/components/"
kernel=kernel/target/debug/rc-kernel
run() { "$kernel" --component-dir "$directory/components" "$@"; }
fixture="smoke-$(date +%s)-$$"
run ssh-policy-seed "$fixture" "$ed" "$rsa" >"$directory/seed.out"
run ssh-policy-verify "$fixture" >"$directory/verify.out"
grep -Fx "ssh policy state: ok" "$directory/verify.out" >/dev/null
python3 - "$directory/kernel.sqlite3" <<'PY'
import sqlite3, sys
rows = sqlite3.connect(sys.argv[1]).execute(
    "SELECT value FROM rc_component_entries WHERE owner=?", ("ohrats:ssh-policy-store",)
).fetchall()
assert rows
for (value,) in rows:
    assert b"PRIVATE KEY" not in value
PY
echo "ssh policy smoke: ok"
