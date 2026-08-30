#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

built_fixtures=0
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in diagnostics-store process-policy shell execution-runtime scheduler transport-webrtc transport-test; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
  cargo build --manifest-path components/process-policy/Cargo.toml --locked --release \
    --target wasm32-wasip2 --features fixture >/dev/null
  cp components/process-policy/target/wasm32-wasip2/release/rc_process_policy.wasm \
    dist/components/process-policy-fixture.wasm
  built_fixtures=1
fi

cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
kernel=kernel/target/debug/rc-kernel
directory=$(mktemp -d)
component_directory="$directory/components"
mkdir -p "$component_directory"
probe_pid=
cleanup() {
  if [ -n "$probe_pid" ]; then kill "$probe_pid" 2>/dev/null || true; fi
  if [ "$built_fixtures" = 1 ]; then
    rm -f dist/components/process-policy-fixture.wasm dist/components/transport-test.wasm
  fi
  rm -rf "$directory"
}
trap cleanup EXIT HUP INT TERM

cp dist/components/process-policy.wasm "$component_directory/"
cp dist/components/diagnostics-store.wasm "$component_directory/"
cp dist/components/shell.wasm "$component_directory/"
cp dist/components/execution-runtime.wasm "$component_directory/"
cp dist/components/scheduler.wasm "$component_directory/"
cp dist/components/transport-webrtc.wasm "$component_directory/"

"$kernel" --component-dir "$component_directory" policy-check \
  | grep -F 'node component policies: ok' >/dev/null
if "$kernel" --component-dir "$component_directory" policy-check >"$directory/replay.out" 2>&1; then
  echo 'execution replay ledger did not survive kernel restart' >&2
  exit 1
fi
grep -F 'was already claimed' "$directory/replay.out" >/dev/null
"$kernel" --component-dir "$component_directory" components > "$directory/graph.out"
grep -F 'ohrats:process-policy' "$directory/graph.out" | grep -F Active >/dev/null
grep -F 'ohrats:execution-runtime' "$directory/graph.out" | grep -F Active >/dev/null
grep -F 'ohrats:scheduler' "$directory/graph.out" | grep -F Active >/dev/null
grep -F 'ohrats:transport-webrtc' "$directory/graph.out" | grep -F Active >/dev/null

mv "$component_directory/process-policy.wasm" "$component_directory/process-policy.disabled"
if "$kernel" --component-dir "$component_directory" policy-check >/dev/null 2>&1; then
  echo 'Node policy check unexpectedly succeeded without process-policy' >&2
  exit 1
fi
mv "$component_directory/process-policy.disabled" "$component_directory/process-policy.wasm"

cp dist/components/transport-test.wasm "$component_directory/"
rm "$component_directory/transport-test.wasm"

fifo="$directory/probe.in"
mkfifo "$fifo"
"$kernel" --component-dir "$component_directory" policy-probe <"$fifo" >"$directory/probe.out" 2>"$directory/probe.err" &
probe_pid=$!
exec 3>"$fifo"
printf '\n' >&3
wait_for_line() {
  expected=$1
  count=0
  while ! grep -F "$expected" "$directory/probe.out" >/dev/null 2>&1; do
    count=$((count + 1))
    if [ "$count" -ge 100 ]; then
      echo "policy probe did not report $expected" >&2
      return 1
    fi
    sleep 0.05
  done
}
wait_for_line 'Host 6000'
printf 'process\n' >&3
wait_for_line 'Process 1048576'
cp dist/components/transport-test.wasm "$component_directory/transport-webrtc.next"
mv "$component_directory/transport-webrtc.next" "$component_directory/transport-webrtc.wasm"
count=0
while ! grep -F 'Relay 50' "$directory/probe.out" >/dev/null 2>&1; do
  printf '\n' >&3
  count=$((count + 1))
  if [ "$count" -ge 1200 ]; then
    echo 'hot replacement did not change the transport attempt plan' >&2
    sed -n '1,40p' "$directory/probe.out" >&2
    sed -n '1,80p' "$directory/probe.err" >&2
    exit 1
  fi
  sleep 0.05
done
cp dist/components/process-policy-fixture.wasm "$component_directory/process-policy.next"
mv "$component_directory/process-policy.next" "$component_directory/process-policy.wasm"
count=0
while ! grep -F 'Process 64' "$directory/probe.out" >/dev/null 2>&1; do
  printf 'process\n' >&3
  count=$((count + 1))
  if [ "$count" -ge 1200 ]; then
    echo 'hot replacement did not change the process policy decision' >&2
    exit 1
  fi
  sleep 0.05
done
exec 3>&-
wait "$probe_pid"

echo 'node components: ok'
