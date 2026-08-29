#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/updater >/dev/null
fi
if [ "${RC_SKIP_KERNEL_BUILD:-0}" != 1 ]; then
  cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
fi

directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
cp dist/components/updater.wasm "$components/updater.wasm"
target="$directory/rc-kernel"
cp kernel/target/debug/rc-kernel "$target"
digest=$(shasum -a 256 fixtures/updater-artifact.sh | awk '{print "sha256:" $1}')
kernel=kernel/target/debug/rc-kernel

RC_UPDATER_ARTIFACT_PATH="$root/fixtures/updater-artifact.sh" RC_NATIVE_TARGET="$target" "$kernel" --component-dir "$components" upgrade "$digest" >"$directory/upgrade.out"
grep -F 'upgraded kernel to 0.1.1' "$directory/upgrade.out" >/dev/null
grep -F 'RC kernel 0.1.1' "$target" >/dev/null
test -f "$directory/.rc-kernel-replacement.journal"
test "$(find "$directory" -maxdepth 1 -name '.rc-kernel-backup-*' | wc -l | tr -d ' ')" -eq 1

RC_UPDATER_ARTIFACT_PATH="$root/fixtures/updater-artifact.sh" RC_NATIVE_TARGET="$target" "$kernel" --component-dir "$components" upgrade "$digest" >"$directory/noop.out"
grep -F 'kernel already at 0.1.1' "$directory/noop.out" >/dev/null
test ! -e "$directory/.rc-kernel-replacement.journal"

echo 'updater smoke: ok'
