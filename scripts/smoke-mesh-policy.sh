#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/mesh-policy >/dev/null
  scripts/build-component.sh components/mesh-policy-fixture >/dev/null
fi
for artifact in mesh-policy mesh-policy-fixture; do
  test -f "dist/components/$artifact.wasm"
done
if [ "${RC_SKIP_KERNEL_BUILD:-0}" != 1 ]; then
  cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
fi

directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT INT TERM
mkdir -p "$directory/components"
cp dist/components/mesh-policy-fixture.wasm "$directory/components/"
kernel=${RC_KERNEL_BIN:-kernel/target/debug/rc-kernel}
run() { "$kernel" --component-dir "$directory/components" "$@"; }

run components >"$directory/waiting.out" 2>/dev/null
grep -F 'ohrats:mesh-policy-fixture' "$directory/waiting.out" | grep -F 'Waiting' >/dev/null

cp dist/components/mesh-policy.wasm "$directory/components/"
run mesh-policy-verify | grep -Fx 'mesh policy fixture: ok' >/dev/null

rm "$directory/components/mesh-policy.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F 'ohrats:mesh-policy-fixture' "$directory/removed.out" | grep -F 'Waiting' >/dev/null

echo 'mesh policy smoke: ok'
