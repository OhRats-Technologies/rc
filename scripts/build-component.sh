#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/build-component.sh <component-directory>" >&2
  exit 2
fi

component=${1%/}
manifest="$component/Cargo.toml"
test -f "$manifest" || {
  echo "missing component manifest: $manifest" >&2
  exit 1
}

rustup target add wasm32-wasip2 >/dev/null
name=$(sed -n 's/^name = "\([^"]*\)"/\1/p' "$manifest" | head -1)
test -n "$name" || {
  echo "could not read package name from $manifest" >&2
  exit 1
}

cargo build --manifest-path "$manifest" --locked --release --target wasm32-wasip2
mkdir -p dist/components
source="$component/target/wasm32-wasip2/release/$(printf '%s' "$name" | tr - _).wasm"
test -f "$source" || {
  echo "component output not found: $source" >&2
  exit 1
}
destination="dist/components/${name#rc-}.wasm"
cp "$source" "$destination"
printf '%s\n' "$destination"
