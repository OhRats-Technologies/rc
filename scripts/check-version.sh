#!/bin/sh
set -eu

package_version="$(sed -n 's/^[[:space:]]*"version": "\([^"]*\)",/\1/p' package.json | head -1)"
cli_version="$(sed -n 's/^version = "\([^"]*\)"/\1/p' crates/rc-cli/Cargo.toml | head -1)"
server_version="$(sed -n 's/^version = "\([^"]*\)"/\1/p' crates/rc-server/Cargo.toml | head -1)"

for value in "$package_version" "$cli_version" "$server_version"; do
  if [ -z "$value" ]; then
    echo "could not read every RC version source" >&2
    exit 1
  fi
done

if [ "$package_version" != "$cli_version" ] || [ "$cli_version" != "$server_version" ]; then
  echo "RC versions differ: package=$package_version cli=$cli_version server=$server_version" >&2
  exit 1
fi

if [ "$#" -gt 0 ] && [ "$1" != "$cli_version" ]; then
  echo "expected RC version $1, found $cli_version" >&2
  exit 1
fi

printf '%s\n' "$cli_version"
