#!/bin/sh
set -eu

if [ "$#" -gt 2 ]; then
  echo "usage: packaging/build-legacy-core-bundle.sh [output] [component-directory]" >&2
  exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
output=${1:-$root/dist/rc-core-components.tar.gz}
source_dir=${2:-$root/dist/components}
case "$output" in
  /*) ;;
  *) output="$root/$output" ;;
esac
case "$source_dir" in
  /*) ;;
  *) source_dir="$root/$source_dir" ;;
esac

# v0.19.2 validates this exact member set and rejects profile.lock/extra files.
components="diagnostics-cli diagnostics-reporter diagnostics-store github-source http-source local-source oci-source package-manager process-policy transport-webrtc"
for name in $components; do
  test -f "$source_dir/$name.wasm" || {
    echo "missing legacy component artifact: $source_dir/$name.wasm" >&2
    exit 1
  }
done

stage=$(mktemp -d "${TMPDIR:-/tmp}/rc-core-legacy.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/components"
for name in $components; do
  cp "$source_dir/$name.wasm" "$stage/components/$name.wasm"
  touch -t 197001010000 "$stage/components/$name.wasm" 2>/dev/null || true
done
touch -t 197001010000 "$stage/components" "$stage" 2>/dev/null || true
mkdir -p "$(dirname -- "$output")"
temporary="$output.rc-bundle.$$"
rm -f "$temporary"
COPYFILE_DISABLE=1 tar -C "$stage" -czf "$temporary" components
mv "$temporary" "$output"
echo "$output"
