#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/crypto-control >/dev/null
fi
test -f dist/components/crypto-control.wasm || {
  echo "missing crypto-control component artifact" >&2
  exit 1
}
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
probe_pid=
cleanup() {
  if [ -n "$probe_pid" ]; then kill "$probe_pid" 2>/dev/null || true; fi
  rm -rf "$directory"
}
trap cleanup EXIT HUP INT TERM
components="$directory/components"
mkdir -p "$components"
kernel=kernel/target/debug/rc-kernel

if "$kernel" --component-dir "$components" crypto-check >/dev/null 2>&1; then
  echo "control crypto check unexpectedly succeeded without provider" >&2
  exit 1
fi
cp dist/components/crypto-control.wasm "$components/crypto-control.wasm"
"$kernel" --component-dir "$components" crypto-check \
  | grep -Fx "control crypto: ok" >/dev/null

key_file=$(find "$directory/state/keys" -type f -name '*.x25519' -print)
test -n "$key_file"
test "$(printf '%s\n' "$key_file" | wc -l | tr -d ' ')" = 1
if stat -f '%Lp' "$key_file" >/dev/null 2>&1; then
  mode=$(stat -f '%Lp' "$key_file")
else
  mode=$(stat -c '%a' "$key_file")
fi
test "$mode" = 600

fifo="$directory/probe.in"
mkfifo "$fifo"
"$kernel" --component-dir "$components" crypto-probe \
  <"$fifo" >"$directory/probe.out" 2>"$directory/probe.err" &
probe_pid=$!
exec 3>"$fifo"

wait_for_line() {
  expected=$1
  count=0
  while ! grep -Fx "$expected" "$directory/probe.out" >/dev/null 2>&1; do
    count=$((count + 1))
    if [ "$count" -ge 200 ]; then
      echo "control crypto probe did not report $expected" >&2
      cat "$directory/probe.out" >&2 || true
      cat "$directory/probe.err" >&2 || true
      exit 1
    fi
    sleep 0.05
  done
}

printf 'open\n' >&3
count=0
while [ "$(grep -c '^Open ' "$directory/probe.out" 2>/dev/null || true)" -lt 1 ]; do
  count=$((count + 1))
  test "$count" -lt 200 || exit 1
  sleep 0.05
done
first=$(grep '^Open ' "$directory/probe.out" | head -n 1 | cut -d' ' -f2)
printf '%s' "$first" | grep -E '^[A-Za-z0-9_-]{43}$' >/dev/null

mv "$components/crypto-control.wasm" "$components/crypto-control.disabled"
count=0
while ! grep -Fx 'Unavailable' "$directory/probe.out" >/dev/null 2>&1; do
  printf 'available\n' >&3
  count=$((count + 1))
  test "$count" -lt 200 || exit 1
  sleep 0.05
done
printf 'roundtrip\n' >&3
wait_for_line 'Roundtrip ok'
printf 'close\n' >&3
wait_for_line 'Closed'

mv "$components/crypto-control.disabled" "$components/crypto-control.wasm"
count=0
while [ "$(grep -c '^Available$' "$directory/probe.out" 2>/dev/null || true)" -lt 2 ]; do
  printf 'available\n' >&3
  count=$((count + 1))
  test "$count" -lt 200 || exit 1
  sleep 0.05
done
printf 'open\n' >&3
count=0
while [ "$(grep -c '^Open ' "$directory/probe.out" 2>/dev/null || true)" -lt 2 ]; do
  count=$((count + 1))
  test "$count" -lt 200 || exit 1
  sleep 0.05
done
second=$(grep '^Open ' "$directory/probe.out" | tail -n 1 | cut -d' ' -f2)
test "$second" = "$first"
printf 'close\n' >&3
exec 3>&-
wait "$probe_pid"
probe_pid=

echo "control crypto smoke: ok"
