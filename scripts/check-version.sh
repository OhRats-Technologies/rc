#!/bin/sh
set -eu

package_version="$(sed -n 's/^[[:space:]]*"version": "\([^"]*\)",/\1/p' package.json | head -1)"
cli_version="$(sed -n 's/^var Version = "\([^"]*\)".*/\1/p' pkg/version/version.go | head -1)"
changelog_version="$(sed -n 's/^## \[\([^]]*\)\].*/\1/p' CHANGELOG.md | head -1)"

for value in "$package_version" "$cli_version" "$changelog_version"; do
  if [ -z "$value" ]; then
    echo "could not read every RC version source" >&2
    exit 1
  fi
done

if [ "$package_version" != "$cli_version" ] || [ "$cli_version" != "$changelog_version" ]; then
  echo "RC versions differ: package=$package_version cli=$cli_version changelog=$changelog_version" >&2
  exit 1
fi

if [ "$#" -gt 0 ] && [ "$1" != "$cli_version" ]; then
  echo "expected RC version $1, found $cli_version" >&2
  exit 1
fi

printf '%s\n' "$cli_version"
