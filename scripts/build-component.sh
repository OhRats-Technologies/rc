#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/build-component.sh <component-directory>" >&2
  exit 2
fi

component=${1%/}
manifest="$component/Cargo.toml"
component_manifest="$component/component.toml"
test -f "$manifest" || {
  echo "missing component manifest: $manifest" >&2
  exit 1
}
test -f "$component_manifest" || {
  echo "missing component manifest: $component_manifest" >&2
  exit 1
}

rustup target add wasm32-wasip2 >/dev/null
values=$(python3 - "$component_manifest" <<'PY'
import sys, tomllib
with open(sys.argv[1], "rb") as handle:
    value = tomllib.load(handle)["build"]
print(value["package"], value["artifact"])
PY
)
package=${values% *}
artifact=${values#* }

cargo build --manifest-path "$manifest" --locked --release --target wasm32-wasip2
mkdir -p dist/components
source="$component/target/wasm32-wasip2/release/$(printf '%s' "$package" | tr - _).wasm"
test -f "$source" || {
  echo "component output not found: $source" >&2
  exit 1
}
destination="dist/components/$artifact.wasm"
cp "$source" "$destination"
printf '%s\n' "$destination"
