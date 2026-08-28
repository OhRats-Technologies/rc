#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

build="local-source http-source oci-source package-manager fixture-provider fixture-provider-v2"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in $build; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi
for artifact in $build; do
  test -f "dist/components/$artifact.wasm" || {
    echo "missing package smoke artifact: $artifact.wasm" >&2
    exit 1
  }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
server_pid=
cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$directory"
}
trap cleanup EXIT INT TERM

components="$directory/components"
mkdir -p "$components" "$directory/catalogs"
for component in local-source http-source oci-source package-manager; do
  cp "dist/components/$component.wasm" "$components/$component.wasm"
done
cp dist/components/fixture-provider.wasm "$directory/demo.wasm"
cp dist/components/fixture-provider.wasm "$directory/v1.wasm"
cp dist/components/fixture-provider-v2.wasm "$directory/v2.wasm"

kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

# A direct local source becomes managed while the source providers stay unmanaged.
run add "$directory/demo.wasm" >"$directory/add.out" 2>/dev/null
grep -F "added ohrats:fixture-provider 1.0.0" "$directory/add.out" >/dev/null
run list >"$directory/list-v1.out" 2>/dev/null
grep -F "demo	ohrats:fixture-provider	1.0.0	true" "$directory/list-v1.out" >/dev/null

state="$directory/state/components/ohrats_package_manager"
test -f "$state/rc.toml"
test -f "$state/rc.lock"
grep -F 'spec = ' "$state/rc.toml" >/dev/null
grep -F 'version = "1.0.0"' "$state/rc.lock" >/dev/null
test "$(find "$directory/cache/sha256" -type f -name '*.wasm' | wc -l | tr -d ' ')" -ge 1

# Exact installation succeeds from the content-addressed cache after the source vanishes.
rm "$directory/demo.wasm" "$components/demo.wasm" "$components/demo.managed"
run install >/dev/null 2>&1
run list >"$directory/list-restored.out" 2>/dev/null
grep -F "demo	ohrats:fixture-provider	1.0.0	true" "$directory/list-restored.out" >/dev/null

cp dist/components/fixture-provider-v2.wasm "$directory/demo.wasm"
run outdated demo >"$directory/outdated.out" 2>/dev/null
grep -F "demo	1.0.0	sha256:" "$directory/outdated.out" >/dev/null
run update demo >"$directory/update.out" 2>/dev/null
grep -F "updated ohrats:fixture-provider 2.0.0" "$directory/update.out" >/dev/null
run remove demo >/dev/null 2>&1

# A declarative catalog selects the target range and exposes a newer major release.
cat >"$directory/catalogs/ohrats.toml" <<EOF
schema = 1
namespace = "ohrats"

[[package]]
name = "fixture-provider"
version = "1.0.0"
source = "$directory/v1.wasm"

[[package]]
name = "fixture-provider"
version = "2.0.0"
source = "$directory/v2.wasm"
EOF
run add 'ohrats:fixture-provider@^1' >/dev/null 2>&1
run outdated fixture-provider >"$directory/catalog-outdated.out" 2>/dev/null
grep -F "fixture-provider	1.0.0	1.0.0	2.0.0" "$directory/catalog-outdated.out" >/dev/null
run update fixture-provider --latest >"$directory/catalog-update.out" 2>/dev/null
grep -F "updated ohrats:fixture-provider 2.0.0" "$directory/catalog-update.out" >/dev/null
grep -F 'spec = "ohrats:fixture-provider@^2"' "$state/rc.toml" >/dev/null
run remove fixture-provider >/dev/null 2>&1

# Network adapters are kernel capabilities; HTTP and OCI resolution remain components.
port_file="$directory/port"
python3 scripts/package-fixture-server.py dist/components/fixture-provider.wasm "$port_file" &
server_pid=$!
count=0
while [ ! -s "$port_file" ]; do
  count=$((count + 1))
  [ "$count" -lt 100 ] || { echo "fixture server did not start" >&2; exit 1; }
  sleep 0.05
done
port=$(cat "$port_file")
run add "http://127.0.0.1:$port/http-demo.wasm" >/dev/null 2>&1
run remove http-demo >/dev/null 2>&1
run add "oci:127.0.0.1:$port/test/component:latest" >/dev/null 2>&1
grep -F 'resolved_source = "oci:127.0.0.1:' "$state/rc.lock" >/dev/null
grep -F '@sha256:' "$state/rc.lock" >/dev/null
run remove oci-demo >/dev/null 2>&1

if run remove local-source >/dev/null 2>&1; then
  echo "package manager removed an unmanaged component" >&2
  exit 1
fi
if run add github:example/project >/dev/null 2>"$directory/github.err"; then
  echo "unregistered source scheme unexpectedly resolved" >&2
  exit 1
fi
grep -F 'package source provider "github" is not installed' "$directory/github.err" >/dev/null

echo "package manager smoke: ok"
