#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/fixture-provider >/dev/null
  scripts/build-component.sh components/fixture-consumer >/dev/null
  scripts/build-component.sh components/fixture-broken >/dev/null
  scripts/build-component.sh components/fixture-collision >/dev/null
  scripts/build-component.sh components/fixture-trap >/dev/null
  scripts/build-component.sh components/fixture-limit >/dev/null
fi
for fixture in provider consumer broken collision trap limit; do
  test -f "dist/components/fixture-$fixture.wasm" || {
    echo "missing fixture artifact: fixture-$fixture.wasm" >&2
    exit 1
  }
done
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
log=$(mktemp)
pid=
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$directory" "$log"
}
trap cleanup EXIT INT TERM

components="$directory/components"
mkdir -p "$components"

wait_for() {
  pattern=$1
  count=0
  while ! grep -F "$pattern" "$log" >/dev/null 2>&1; do
    count=$((count + 1))
    if [ "$count" -ge 100 ]; then
      echo "timed out waiting for: $pattern" >&2
      cat "$log" >&2
      exit 1
    fi
    sleep 0.05
  done
}

cp dist/components/fixture-consumer.wasm "$components/consumer.wasm"
kernel/target/debug/rc-kernel --component-dir "$components" watch >"$log" 2>&1 &
pid=$!
wait_for "ohrats:fixture-consumer  1.0.0        Waiting"

cp dist/components/fixture-provider.wasm "$components/provider.wasm"
wait_for "ohrats:fixture-provider  1.0.0        Active"
wait_for "ohrats:fixture-consumer  1.0.0        Active"

cp dist/components/fixture-collision.wasm "$components/collision.wasm"
wait_for 'command "hello" is already provided by ohrats:fixture-provider'

cp dist/components/fixture-broken.wasm "$components/provider.new"
mv "$components/provider.new" "$components/provider.wasm"
wait_for "replacement activation failed: intentional activation failure"
wait_for "ohrats:fixture-provider  1.0.0        Active"

rm "$components/provider.wasm"
wait_for "ohrats:fixture-consumer  1.0.0        Waiting"

kill "$pid"
wait "$pid" 2>/dev/null || true
pid=

rm -f "$components/consumer.wasm" "$components/collision.wasm"
cp dist/components/fixture-provider.wasm "$components/provider.wasm"
output=$(kernel/target/debug/rc-kernel --component-dir "$components" hello RC 2>/dev/null)
test "$output" = "hello, RC"
cp dist/components/fixture-consumer.wasm "$components/consumer.wasm"
output=$(kernel/target/debug/rc-kernel --component-dir "$components" consume WIT 2>/dev/null)
test "$output" = "hello, WIT"

rm -f "$components/provider.wasm" "$components/consumer.wasm"
cp dist/components/fixture-trap.wasm "$components/trap.wasm"
if kernel/target/debug/rc-kernel --component-dir "$components" repair >/dev/null 2>&1; then
  echo "trapping component unexpectedly passed repair" >&2
  exit 1
fi
rm "$components/trap.wasm"
cp dist/components/fixture-limit.wasm "$components/limit.wasm"
if kernel/target/debug/rc-kernel --component-dir "$components" repair >/dev/null 2>&1; then
  echo "memory-limit component unexpectedly passed repair" >&2
  exit 1
fi

echo "kernel component smoke: ok"
