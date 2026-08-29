#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

build="artifact-cache-local artifact-cache-mesh artifact-cache-fixture-mesh-adapter artifact-cache-fixture-consumer artifact-cache-fixture-provider-v2"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in $build; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi
for artifact in $build; do
  test -f "dist/components/$artifact.wasm"
done
if [ "${RC_SKIP_KERNEL_BUILD:-0}" != 1 ]; then
  cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
fi

directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
for component in artifact-cache-local artifact-cache-mesh artifact-cache-fixture-mesh-adapter artifact-cache-fixture-consumer; do
  cp "dist/components/$component.wasm" "$components/$component.wasm"
done
kernel=${RC_KERNEL_BIN:-kernel/target/debug/rc-kernel}
test -x "$kernel"
run() { "$kernel" --component-dir "$components" "$@"; }

# Seed the kernel-owned content-addressed cache used by the production local
# provider. The digest is SHA-256("local-cache-hit").
mkdir -p "$directory/cache/sha256"
printf 'local-cache-hit' > "$directory/cache/sha256/3071910d02cb4b93c5bf83d2f04eabbd1b1f25062ca6f161e8f60453c96b1f48.wasm"
printf 'tampered-cache-hit' > "$directory/cache/sha256/5d4553c6b682104be56b7da1ddf97ae3913e29432fb169a13a2310cc861dc36f.wasm"

run cache-local >"$directory/local.out" 2>"$directory/local.err"
grep -Fx "local:local-cache-hit" "$directory/local.out" >/dev/null
run cache-fallback >"$directory/fallback.out" 2>"$directory/fallback.err"
grep -Fx "mesh:mesh-cache-hit" "$directory/fallback.out" >/dev/null
run cache-miss >"$directory/miss.out" 2>"$directory/miss.err"
grep -Fx "registry:miss" "$directory/miss.out" >/dev/null

if run cache-unauthorized >"$directory/unauthorized.out" 2>"$directory/unauthorized.err"; then
  echo "unauthorized mesh cache request succeeded" >&2
  exit 1
fi
grep -F "mesh cache request is not authorized" "$directory/unauthorized.err" >/dev/null
if run cache-tampered >"$directory/tampered.out" 2>"$directory/tampered.err"; then
  echo "tampered cache artifact succeeded" >&2
  exit 1
fi
grep -F "artifact digest mismatch" "$directory/tampered.err" >/dev/null
if run cache-oversized >"$directory/oversized.out" 2>"$directory/oversized.err"; then
  echo "oversized cache request succeeded" >&2
  exit 1
fi
grep -F "between 1 and 48 MiB" "$directory/oversized.err" >/dev/null

run cache-priority >"$directory/priority-base.out" 2>/dev/null
test "$(sed -n '1p' "$directory/priority-base.out" | cut -f1)" = "ohrats:artifact-cache-local"
test "$(sed -n '2p' "$directory/priority-base.out" | cut -f1)" = "ohrats:artifact-cache-mesh"
run cache-replacement | grep -Fx "replacement:miss" >/dev/null

cp dist/components/artifact-cache-fixture-provider-v2.wasm "$components/replacement.wasm"
run cache-priority >"$directory/priority-replaced.out" 2>/dev/null
test "$(sed -n '1p' "$directory/priority-replaced.out" | cut -f1)" = "ohrats:artifact-cache-fixture-provider-v2"
run cache-replacement | grep -Fx "replacement:hit" >/dev/null
rm "$components/replacement.wasm"
run cache-replacement | grep -Fx "replacement:miss" >/dev/null

rm "$components/artifact-cache-local.wasm"
run cache-priority >"$directory/priority-withdrawn.out" 2>/dev/null
if grep -F "ohrats:artifact-cache-local" "$directory/priority-withdrawn.out" >/dev/null; then
  echo "withdrawn local provider remained in registry" >&2
  exit 1
fi
if run cache-fallback >"$directory/mesh-only.out" 2>"$directory/mesh-only.err"; then
  echo "local-key invocation succeeded after local provider withdrawal" >&2
  exit 1
fi
grep -F "service ohrats:rc-artifact-cache/cache" "$directory/mesh-only.err" >/dev/null

echo "artifact cache runtime smoke: ok"
