#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

rustup target add wasm32-wasip2 >/dev/null

for manifest in components/*/Cargo.toml; do
  component=${manifest%/Cargo.toml}
  cargo fmt --manifest-path "$manifest" --all -- --check
  cargo clippy \
    --manifest-path "$manifest" \
    --target wasm32-wasip2 \
    --all-targets \
    -- -D warnings
  scripts/build-component.sh "$component" >/dev/null
done

echo "components: ok"
