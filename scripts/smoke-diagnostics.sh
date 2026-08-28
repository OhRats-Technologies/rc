#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

build="diagnostics-store diagnostics-cli diagnostics-reporter diagnostics-ui diagnostics-mesh webui-shell"
if [ "${RC_SKIP_COMPONENT_BUILD:-0}" != 1 ]; then
  for component in $build; do
    scripts/build-component.sh "components/$component" >/dev/null
  done
fi
cargo build --manifest-path kernel/Cargo.toml --locked >/dev/null

directory=$(mktemp -d)
watch_pid=
cleanup() {
  if [ -n "$watch_pid" ]; then
    kill "$watch_pid" 2>/dev/null || true
    wait "$watch_pid" 2>/dev/null || true
  fi
  rm -rf "$directory"
}
trap cleanup EXIT INT TERM

components="$directory/components"
mkdir -p "$components"
for component in diagnostics-store diagnostics-cli diagnostics-reporter diagnostics-ui diagnostics-mesh; do
  cp "dist/components/$component.wasm" "$components/$component.wasm"
done
kernel=kernel/target/debug/rc-kernel
run() {
  "$kernel" --component-dir "$components" "$@"
}

run components >"$directory/without-webui.out" 2>/dev/null
grep -F "ohrats:diagnostics-store" "$directory/without-webui.out" | grep -F Active >/dev/null
grep -F "ohrats:diagnostics-cli" "$directory/without-webui.out" | grep -F Active >/dev/null
grep -F "ohrats:diagnostics-reporter" "$directory/without-webui.out" | grep -F Active >/dev/null
grep -F "ohrats:diagnostics-ui" "$directory/without-webui.out" | grep -F Waiting >/dev/null
grep -F "ohrats:diagnostics-mesh" "$directory/without-webui.out" | grep -F Waiting >/dev/null

run doctor >"$directory/doctor.out" 2>/dev/null
grep -F "RC diagnostics" "$directory/doctor.out" >/dev/null
grep -F "retained 1" "$directory/doctor.out" >/dev/null
run logs 5 >"$directory/logs.out" 2>/dev/null
grep -F "component.active: diagnostics reporter activated" "$directory/logs.out" >/dev/null

cp dist/components/webui-shell.wasm "$components/webui-shell.wasm"
run ui-pages >"$directory/pages.out" 2>/dev/null
grep -F "diagnostics	/diagnostics	Diagnostics" "$directory/pages.out" >/dev/null

rm "$components/diagnostics-store.wasm"
run components >"$directory/without-store.out" 2>/dev/null
grep -F "ohrats:diagnostics-cli" "$directory/without-store.out" | grep -F Waiting >/dev/null
grep -F "ohrats:diagnostics-reporter" "$directory/without-store.out" | grep -F Waiting >/dev/null
grep -F "ohrats:diagnostics-ui" "$directory/without-store.out" | grep -F Waiting >/dev/null
if run ui-pages 2>/dev/null | grep -F diagnostics >/dev/null; then
  echo "diagnostics UI activated without diagnostics query" >&2
  exit 1
fi
cp dist/components/diagnostics-store.wasm "$components/diagnostics-store.wasm"
run ui-pages 2>/dev/null | grep -F "diagnostics	/diagnostics" >/dev/null

# Prove the same dependency transition while one kernel process keeps watching.
rm "$components/webui-shell.wasm"
watch_log="$directory/watch.log"
"$kernel" --component-dir "$components" watch >"$watch_log" 2>&1 &
watch_pid=$!
state_count() {
  component=$1
  state=$2
  awk -v component="$component" -v state="$state" '
    index($0, component) && index($0, state) { count += 1 }
    END { print count + 0 }
  ' "$watch_log"
}
wait_for_state() {
  component=$1
  state=$2
  minimum=$3
  count=0
  while [ "$(state_count "$component" "$state")" -lt "$minimum" ]; do
    count=$((count + 1))
    [ "$count" -lt 120 ] || { cat "$watch_log" >&2; return 1; }
    sleep 0.05
  done
}
wait_for_state "ohrats:diagnostics-ui" Waiting 1
cp dist/components/webui-shell.wasm "$components/webui-shell.wasm"
wait_for_state "ohrats:diagnostics-ui" Active 1
rm "$components/webui-shell.wasm"
wait_for_state "ohrats:diagnostics-ui" Waiting 2

echo "diagnostics smoke: ok"
