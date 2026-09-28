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
kernel=kernel/target/debug/rc-kernel
next_version=$("$kernel" --version | awk '{split($NF, v, "."); printf "%d.%d.%d", v[1], v[2], v[3]+1}')
artifact="$directory/updater-artifact.sh"
sed "s/RC kernel [0-9.]*/RC kernel $next_version/" fixtures/updater-artifact.sh >"$artifact"
digest=$(shasum -a 256 "$artifact" | awk '{print "sha256:" $1}')

RC_UPDATER_ARTIFACT_PATH="$artifact" RC_NATIVE_TARGET="$target" "$kernel" --component-dir "$components" upgrade "$digest" >"$directory/upgrade.out"
grep -F "upgraded kernel to $next_version" "$directory/upgrade.out" >/dev/null
grep -F "RC kernel $next_version" "$target" >/dev/null
test -f "$directory/.rc-kernel-replacement.journal"
test "$(find "$directory" -maxdepth 1 -name '.rc-kernel-backup-*' | wc -l | tr -d ' ')" -eq 1

RC_UPDATER_ARTIFACT_PATH="$artifact" RC_NATIVE_TARGET="$target" "$kernel" --component-dir "$components" upgrade "$digest" >"$directory/noop.out"
grep -F "kernel already at $next_version" "$directory/noop.out" >/dev/null
test ! -e "$directory/.rc-kernel-replacement.journal"

echo 'updater smoke: ok'
