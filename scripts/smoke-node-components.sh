#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in process-policy transport-webrtc transport-test; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi

cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null
kernel=kernel/target/debug/rc-kernel
directory=$(mktemp -d)
probe_pid=
cleanup() {
  if [ -n "$probe_pid" ]; then kill "$probe_pid" 2>/dev/null || true; fi
  rm -rf "$directory"
}
trap cleanup EXIT HUP INT TERM

cp dist/components/process-policy.wasm "$directory/"
cp dist/components/transport-webrtc.wasm "$directory/"

"$kernel" --component-dir "$directory" policy-check \
  | grep -F 'node component policies: ok' >/dev/null
"$kernel" --component-dir "$directory" components > "$directory/graph.out"
grep -F 'ohrats:process-policy' "$directory/graph.out" | grep -F Active >/dev/null
grep -F 'ohrats:transport-webrtc' "$directory/graph.out" | grep -F Active >/dev/null

mv "$directory/process-policy.wasm" "$directory/process-policy.disabled"
if "$kernel" --component-dir "$directory" policy-check >/dev/null 2>&1; then
  echo 'Node policy check unexpectedly succeeded without process-policy' >&2
  exit 1
fi
mv "$directory/process-policy.disabled" "$directory/process-policy.wasm"

cp dist/components/transport-test.wasm "$directory/"
rm "$directory/transport-test.wasm"

fifo="$directory/probe.in"
mkfifo "$fifo"
"$kernel" --component-dir "$directory" policy-probe <"$fifo" >"$directory/probe.out" 2>"$directory/probe.err" &
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
cp dist/components/transport-test.wasm "$directory/transport-webrtc.next"
mv "$directory/transport-webrtc.next" "$directory/transport-webrtc.wasm"
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
exec 3>&-
wait "$probe_pid"

echo 'node components: ok'
