#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/build-component.sh <component-directory>" >&2
  exit 2
fi

component=${1%/}
component_manifest="$component/component.toml"
test -f "$component_manifest" || {
  echo "missing component manifest: $component_manifest" >&2
  exit 1
}

values=$(python3 - "$component_manifest" <<'PY'
import sys, tomllib
with open(sys.argv[1], "rb") as handle:
    data = tomllib.load(handle)
build = data["build"]
if build["kind"] != "go":
    raise SystemExit(f"unsupported component build kind: {build['kind']}")
print(build["artifact"], data["component"]["world"])
PY
)
artifact=$(printf '%s' "$values" | cut -d' ' -f1)
world=$(printf '%s' "$values" | cut -d' ' -f2)

command -v componentize-go >/dev/null 2>&1 || {
  echo "componentize-go is required to build Go components" >&2
  echo "install it with: go install github.com/bytecodealliance/componentize-go@v0.5.3" >&2
  exit 1
}

mkdir -p dist/components
destination="dist/components/$artifact.wasm"

(
  cd "$component"
  componentize-go -d ../../wit -w "$world" build -o "../../$destination"
)
printf '%s\n' "$destination"
