#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in process-policy transport-webrtc transport-test; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi

cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
kernel=kernel/target/debug/rc-kernel
directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT HUP INT TERM

cp dist/components/process-policy.wasm "$directory/"
cp dist/components/transport-webrtc.wasm "$directory/"

"$kernel" --component-dir "$directory" policy-check \
  | grep -F 'node component policies: ok' >/dev/null
"$kernel" --component-dir "$directory" components > "$directory/graph.out"
grep -F 'ohrats:process-policy' "$directory/graph.out" | grep -F Active >/dev/null
grep -F 'ohrats:transport-webrtc' "$directory/graph.out" | grep -F Active >/dev/null

mv "$directory/process-policy.wasm" "$directory/process-policy.disabled"
if "$kernel" --component-dir "$directory" policy-check >/dev/null 2>&1; then
  echo 'Node policy check unexpectedly succeeded without process-policy' >&2
  exit 1
fi
mv "$directory/process-policy.disabled" "$directory/process-policy.wasm"

cp dist/components/transport-test.wasm "$directory/"
rm "$directory/transport-webrtc.wasm"
if "$kernel" --component-dir "$directory" policy-check >/dev/null 2>&1; then
  echo 'WebRTC policy check unexpectedly selected the test transport' >&2
  exit 1
fi

echo 'node components: ok'
