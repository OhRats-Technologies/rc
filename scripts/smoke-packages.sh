#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/local-source >/dev/null
  scripts/build-component.sh components/package-manager >/dev/null
  scripts/build-component.sh components/fixture-provider >/dev/null
  scripts/build-component.sh components/fixture-provider-v2 >/dev/null
fi
for artifact in local-source package-manager fixture-provider fixture-provider-v2; do
  test -f "dist/components/$artifact.wasm" || {
    echo "missing package smoke artifact: $artifact.wasm" >&2
    exit 1
  }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
cp dist/components/local-source.wasm "$components/local-source.wasm"
cp dist/components/package-manager.wasm "$components/package-manager.wasm"
cp dist/components/fixture-provider.wasm "$directory/demo.wasm"

kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

run add "$directory/demo.wasm" >"$directory/add.out"
grep -F "added ohrats:fixture-provider 1.0.0" "$directory/add.out" >/dev/null
run list >"$directory/list-v1.out" 2>/dev/null
grep -F "demo	ohrats:fixture-provider	1.0.0	true" "$directory/list-v1.out" >/dev/null

state="$directory/state/components/ohrats_package_manager"
test -f "$state/rc.toml"
test -f "$state/rc.lock"
grep -F 'source = "file:' "$state/rc.toml" >/dev/null
grep -F 'version = "1.0.0"' "$state/rc.lock" >/dev/null

cp dist/components/fixture-provider-v2.wasm "$directory/demo.wasm"
run outdated demo >"$directory/outdated.out" 2>/dev/null
grep -F "demo	sha256:" "$directory/outdated.out" >/dev/null
run update demo >"$directory/update.out" 2>/dev/null
grep -F "updated ohrats:fixture-provider 2.0.0" "$directory/update.out" >/dev/null
run list >"$directory/list-v2.out" 2>/dev/null
grep -F "demo	ohrats:fixture-provider	2.0.0	true" "$directory/list-v2.out" >/dev/null
grep -F 'version = "2.0.0"' "$state/rc.lock" >/dev/null

run install >/dev/null 2>&1
run remove demo >"$directory/remove.out" 2>/dev/null
grep -F "removed demo" "$directory/remove.out" >/dev/null
if run remove local-source >/dev/null 2>&1; then
  echo "package manager removed an unmanaged component" >&2
  exit 1
fi
if run add github:example/project >/dev/null 2>"$directory/github.err"; then
  echo "unregistered source scheme unexpectedly resolved" >&2
  exit 1
fi
grep -F "service ohrats:rc-plugin/package-source" "$directory/github.err" >/dev/null

echo "package manager smoke: ok"
