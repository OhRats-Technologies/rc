#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

python3 scripts/validate-components.py

for manifest in components/*/component.toml; do
  name=${manifest#components/}
  name=${name%/component.toml}
  scripts/check-component.sh "$name"
done

echo "components: ok"
