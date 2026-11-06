#!/bin/sh
set -eu

IMAGE="${1:-rc-ci}"
NAME="rc-container-smoke-$$"
VOLUME="rc-container-smoke-data-$$"
SETUP_TOKEN="container-smoke-setup-token"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker volume create "$VOLUME" >/dev/null
docker run -d --name "$NAME" \
  -p 127.0.0.1::3000 \
  -v "$VOLUME:/data" \
  -e PUBLIC_URL=http://localhost:3000 \
  -e RC_SETUP_TOKEN="$SETUP_TOKEN" \
  "$IMAGE" >/dev/null

refresh_base() {
  PORT="$(docker inspect "$NAME" --format '{{(index (index .NetworkSettings.Ports "3000/tcp") 0).HostPort}}')"
  BASE="http://127.0.0.1:$PORT"
}

refresh_base

wait_for_health() {
  attempt=0
  while [ "$attempt" -lt 80 ]; do
    if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then
      return 0
    fi
    if ! docker inspect "$NAME" --format '{{.State.Running}}' | grep -qx true; then
      docker logs "$NAME" >&2 || true
      return 1
    fi
    attempt=$((attempt + 1))
    sleep 0.25
  done
  docker logs "$NAME" >&2 || true
  return 1
}

wait_for_health
test "$(curl -fsS "$BASE/healthz")" = "ok"
curl -fsS "$BASE/api/v1/health" | grep -q '"ok":true'
curl -fsS "$BASE/" | grep -q 'RC / REMOTE CONTROL'
curl -fsS "$BASE/docs" | grep -q '<title>'
curl -fsS "$BASE/assets/$(docker exec "$NAME" sh -c 'ls /app/assets | head -1')" >/dev/null

# First-instance setup is token-gated end to end.
test "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/setup/wrong-token")" = "404"
test "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/setup/$SETUP_TOKEN")" = "303"
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
  -d '{"name":"Alice"}' "$BASE/api/v1/auth/setup/options")" = "403"
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
  -H "X-RC-Setup-Token: $SETUP_TOKEN" -d '{"name":"Alice"}' \
  "$BASE/api/v1/auth/setup/options")" = "200"

# Product routes must fail closed until the signed control plane exists.
test "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/devices")" = "401"
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE/mcp")" = "401"

test "$(docker exec "$NAME" id -u rc)" = "10001"
test "$(docker exec "$NAME" id -g rc)" = "10001"
docker exec "$NAME" sh -c 'test "$(stat -c %a /data)" = 700'
docker exec "$NAME" /usr/local/bin/rc --version | grep -q '^RC '
docker exec "$NAME" /usr/local/bin/rc-server --version | grep -q '^RC server '
docker exec "$NAME" sh -c 'ps -eo user=,args= | grep -Eq "^rc[[:space:]]+.*/rc-server$"'
docker exec "$NAME" /usr/local/bin/rc-server --healthcheck

# State survives a restart with the same private-directory permissions.
docker exec "$NAME" sh -c 'printf marker > /data/container-smoke-marker'
docker restart "$NAME" >/dev/null
refresh_base
wait_for_health
test "$(docker exec "$NAME" cat /data/container-smoke-marker)" = "marker"
test "$(docker exec "$NAME" stat -c %a /data)" = "700"

STATUS="$(docker inspect "$NAME" --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}')"
test "$STATUS" = "healthy" || test "$STATUS" = "starting"
echo "container smoke: ok"
