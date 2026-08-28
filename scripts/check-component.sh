#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/check-component.sh <component-name>" >&2
  exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
component="components/$1"
manifest="$component/Cargo.toml"
test -f "$manifest" || {
  echo "unknown component: $1" >&2
  exit 1
}

rustup target add wasm32-wasip2 >/dev/null
cargo fmt --manifest-path "$manifest" --all -- --check
cargo clippy \
  --manifest-path "$manifest" \
  --target wasm32-wasip2 \
  --all-targets \
  -- -D warnings
scripts/build-component.sh "$component" >/dev/null
echo "$1: ok"
