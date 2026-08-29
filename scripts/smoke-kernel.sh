#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

for fixture in \
  fixture-provider \
  fixture-consumer \
  call-context-consumer \
  fixture-broken \
  fixture-collision \
  fixture-trap \
  fixture-limit
do
  if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ] ||
    [ ! -f "dist/components/$fixture.wasm" ]; then
    scripts/build-component.sh "components/$fixture" >/dev/null
  fi
done
for fixture in provider consumer broken collision trap limit; do
  test -f "dist/components/fixture-$fixture.wasm" || {
    echo "missing fixture artifact: fixture-$fixture.wasm" >&2
    exit 1
  }
done
test -f "dist/components/call-context-consumer.wasm" || {
  echo "missing fixture artifact: call-context-consumer.wasm" >&2
  exit 1
}
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
    if [ "$count" -ge 400 ]; then
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

# Invalid bytes at an existing path must not withdraw the healthy generation.
: >"$components/provider.new"
mv "$components/provider.new" "$components/provider.wasm"
wait_for "failed to compile component"
wait_for "ohrats:fixture-provider  1.0.0        Active"

cp dist/components/fixture-broken.wasm "$components/provider.new"
mv "$components/provider.new" "$components/provider.wasm"
wait_for "replacement activation failed: intentional activation failure"
wait_for "ohrats:fixture-provider  1.0.0        Active"

# A valid atomic replacement restores the healthy on-disk generation.
active_count=$(grep -Fc "ohrats:fixture-provider  1.0.0        Active" "$log")
cp dist/components/fixture-provider.wasm "$components/provider.new"
mv "$components/provider.new" "$components/provider.wasm"
count=0
while [ "$(grep -Fc "ohrats:fixture-provider  1.0.0        Active" "$log")" -le "$active_count" ]; do
  count=$((count + 1))
  [ "$count" -lt 400 ] || { cat "$log" >&2; exit 1; }
  sleep 0.05
done

rm "$components/collision.wasm"
rm "$components/provider.wasm"
wait_for "ohrats:fixture-consumer  1.0.0        Waiting"
if kernel/target/debug/rc-kernel --component-dir "$components" hello RC >/dev/null 2>&1; then
  echo "removed provider command remained available" >&2
  exit 1
fi

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
output=$(kernel/target/debug/rc-kernel --component-dir "$components" caller 2>/dev/null)
test "$output" = "ohrats:fixture-consumer"
cp dist/components/call-context-consumer.wasm "$components/caller-alt.wasm"
output=$(kernel/target/debug/rc-kernel --component-dir "$components" caller-alt 2>/dev/null)
test "$output" = "ohrats:call-context-consumer"
rm "$components/consumer.wasm" "$components/caller-alt.wasm"
output=$(kernel/target/debug/rc-kernel --component-dir "$components" provider-caller 2>/dev/null)
test "$output" = "none"

rm -f "$components/provider.wasm"
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
