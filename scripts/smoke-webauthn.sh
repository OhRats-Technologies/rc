#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/webauthn-es256 >/dev/null
  scripts/build-component.sh components/webauthn-fixture >/dev/null
fi
for artifact in webauthn-es256 webauthn-fixture; do
  test -f "dist/components/$artifact.wasm" || {
    echo "missing $artifact component artifact" >&2
    exit 1
  }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
cleanup() { rm -rf "$directory"; }
trap cleanup EXIT INT TERM
components="$directory/components"
mkdir -p "$components"
cp dist/components/webauthn-fixture.wasm "$components/webauthn-fixture.wasm"
kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

run components >"$directory/waiting.out" 2>/dev/null
grep -F "ohrats:webauthn-fixture" "$directory/waiting.out" | grep -F Waiting >/dev/null
if run webauthn-check >/dev/null 2>&1; then
  echo "WebAuthn fixture activated without a verifier" >&2
  exit 1
fi

cp dist/components/webauthn-es256.wasm "$components/webauthn-es256.wasm"
run components >"$directory/active.out" 2>/dev/null
grep -F "ohrats:webauthn-es256" "$directory/active.out" | grep -F Active >/dev/null
grep -F "ohrats:webauthn-fixture" "$directory/active.out" | grep -F Active >/dev/null
run webauthn-check >"$directory/check.out" 2>/dev/null
grep -Fx "webauthn verifier: ok" "$directory/check.out" >/dev/null

rm "$components/webauthn-es256.wasm"
run components >"$directory/removed.out" 2>/dev/null
grep -F "ohrats:webauthn-fixture" "$directory/removed.out" | grep -F Waiting >/dev/null

echo "webauthn smoke: ok"
