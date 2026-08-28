#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

rustup target add wasm32-wasip2 >/dev/null
python3 scripts/validate-components.py

for manifest in components/*/Cargo.toml; do
  name=${manifest#components/}
  name=${name%/Cargo.toml}
  scripts/check-component.sh "$name"
done

echo "components: ok"
