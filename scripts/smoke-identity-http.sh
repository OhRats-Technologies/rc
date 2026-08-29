#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
for component in identity-store webauthn-es256 webui-shell identity-http; do
  scripts/build-component.sh "components/$component" >/dev/null
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
bun scripts/identity-http-e2e.ts
echo "identity HTTP smoke: ok"
