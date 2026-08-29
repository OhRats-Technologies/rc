#!/bin/sh
set -eu

if [ "$#" -gt 2 ]; then
  echo "usage: packaging/build-core-bundle.sh [output] [component-directory]" >&2
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

components="diagnostics-cli diagnostics-reporter diagnostics-store github-source http-source local-source oci-source package-manager process-policy transport-webrtc"
for name in $components; do
  test -f "$source_dir/$name.wasm" || {
    echo "missing component artifact: $source_dir/$name.wasm" >&2
    exit 1
  }
done

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required" >&2
    exit 1
  fi
}

stage=$(mktemp -d "${TMPDIR:-/tmp}/rc-core-bundle.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/components"
printf 'schema 1\nprofile ohrats:core\n' > "$stage/profile.lock"
for name in $components; do
  cp "$source_dir/$name.wasm" "$stage/components/$name.wasm"
  printf 'component %s sha256:%s\n' "$name" "$(hash_file "$stage/components/$name.wasm")" \
    >> "$stage/profile.lock"
done

touch -t 197001010000 "$stage/profile.lock" "$stage/components" \
  "$stage" 2>/dev/null || true
for name in $components; do
  touch -t 197001010000 "$stage/components/$name.wasm" 2>/dev/null || true
done
mkdir -p "$(dirname -- "$output")"
temporary="$output.rc-bundle.$$"
rm -f "$temporary"
COPYFILE_DISABLE=1 tar -C "$stage" -czf "$temporary" profile.lock components
mv "$temporary" "$output"
echo "$output"
