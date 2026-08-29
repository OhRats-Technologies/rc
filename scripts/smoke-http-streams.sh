#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  scripts/build-component.sh components/http-stream-fixture >/dev/null
  scripts/build-component.sh components/http-stream-finite >/dev/null
fi
kernel_target=${RC_KERNEL_TARGET_DIR:-kernel/target}
cargo build --manifest-path kernel/Cargo.toml --locked --offline \
  --target-dir "$kernel_target" >/dev/null

directory=$(mktemp -d)
log=$(mktemp)
pid=
slow_pid=
event_pid=
limit_pid_a=
limit_pid_b=
cleanup() {
  test -z "$limit_pid_b" || kill "$limit_pid_b" 2>/dev/null || true
  test -z "$limit_pid_a" || kill "$limit_pid_a" 2>/dev/null || true
  test -z "$event_pid" || kill "$event_pid" 2>/dev/null || true
  test -z "$slow_pid" || kill "$slow_pid" 2>/dev/null || true
  test -z "$pid" || kill "$pid" 2>/dev/null || true
  test -z "$pid" || wait "$pid" 2>/dev/null || true
  rm -rf "$directory" "$log"
}
trap cleanup EXIT INT TERM

mkdir -p "$directory/components"
cp dist/components/http-stream-fixture.wasm "$directory/components/stream.wasm"
cp dist/components/http-stream-finite.wasm "$directory/components/finite.wasm"
RC_DATA_DIR="$directory/data" RC_HTTP_STREAM_MAX_ACTIVE=2 RC_HTTP_STREAM_MAX_TOTAL=96 \
  RC_HTTP_STREAM_MAX_SESSION_MS=5000 \
  "$kernel_target/debug/rc-kernel" \
  --component-dir "$directory/components" serve \
  --listen 127.0.0.1:32179 >"$log" 2>&1 &
pid=$!

count=0
until curl -fsS http://127.0.0.1:32179/healthz >/dev/null 2>&1; do
  count=$((count + 1))
  test "$count" -lt 400 || { cat "$log" >&2; exit 1; }
  sleep 0.05
done

events=$(curl -fsS --no-buffer http://127.0.0.1:32179/events)
expected='id: 1
data: first

: heartbeat

id: 2
data: second'
test "$events" = "$expected"
test "$(curl -fsS http://127.0.0.1:32179/finite)" = finite

curl -fsS --no-buffer http://127.0.0.1:32179/endless >/dev/null &
limit_pid_a=$!
curl -fsS --no-buffer http://127.0.0.1:32179/endless >/dev/null &
limit_pid_b=$!
sleep 0.1
test "$(curl -sS --max-time 0.5 -o /dev/null -w '%{http_code}' http://127.0.0.1:32179/endless)" = 503
test "$(curl -fsS --max-time 0.15 http://127.0.0.1:32179/finite)" = finite
kill "$limit_pid_a"
wait "$limit_pid_a" 2>/dev/null || true
limit_pid_a=
sleep 0.08
closed_after_limit=$(curl -fsS http://127.0.0.1:32179/closed)
test "${closed_after_limit#data: }" -ge 2
replacement_status=$(curl -sS --max-time 0.12 -o /dev/null -w '%{http_code}' \
  http://127.0.0.1:32179/endless || true)
test "$replacement_status" = 200
kill "$limit_pid_b"
wait "$limit_pid_b" 2>/dev/null || true
limit_pid_b=
# The timed-out replacement stream is released asynchronously by the producer's
# bounded disconnect check. Give it one full check interval before consuming both slots.
sleep 0.08

curl -fsS --no-buffer http://127.0.0.1:32179/events >/dev/null &
event_pid=$!
test "$(curl -fsS --no-buffer http://127.0.0.1:32179/events)" = "$expected"
wait "$event_pid"
event_pid=

curl -fsS http://127.0.0.1:32179/slow >/dev/null &
slow_pid=$!
sleep 0.03
test "$(curl -fsS --max-time 0.15 http://127.0.0.1:32179/finite)" = finite
wait "$slow_pid"
slow_pid=

curl -fsS --max-time 0.12 http://127.0.0.1:32179/endless >/dev/null 2>&1 || true
sleep 0.08
closed=$(curl -fsS http://127.0.0.1:32179/closed)
test "${closed#data: }" != "0"

if curl -fsS http://127.0.0.1:32179/total >/dev/null 2>&1; then
  echo "total stream limit unexpectedly succeeded" >&2
  exit 1
fi

if curl -fsS http://127.0.0.1:32179/oversized >/dev/null 2>&1; then
  echo "oversized stream unexpectedly succeeded" >&2
  exit 1
fi
if curl -fsS http://127.0.0.1:32179/failure >/dev/null 2>&1; then
  echo "failed provider stream unexpectedly succeeded" >&2
  exit 1
fi

rm "$directory/components/stream.wasm"
sleep 0.2
test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:32179/events)" = 404
test "$(curl -fsS http://127.0.0.1:32179/finite)" = finite

echo "kernel streamed HTTP smoke: ok"
