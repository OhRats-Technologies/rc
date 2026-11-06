#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/check-component.sh <component-name>" >&2
  exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
component="components/$1"
component_manifest="$component/component.toml"
test -f "$component_manifest" || {
  echo "unknown component: $1" >&2
  exit 1
}

test -f "$component/go.mod" || {
  echo "$component: missing go.mod" >&2
  exit 1
}

unformatted=$(gofmt -l "$component")
if [ -n "$unformatted" ]; then
  echo "component sources are not gofmt'd:" >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi

(
  cd "$component"
  go vet -unsafeptr=false -composites=false ./...
  go test ./...
)
scripts/build-component.sh "$component" >/dev/null
echo "$1: ok"
