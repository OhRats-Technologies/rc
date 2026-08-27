#!/bin/sh
set -eu

IMAGE="${1:-rc-ci}"
NAME="rc-container-smoke-$$"
VOLUME="rc-container-smoke-data-$$"
TOKEN="container-smoke-setup-token-0123456789abcdef"

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
  -e RC_SETUP_TOKEN="$TOKEN" \
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
curl -fsS "$BASE/api/v1/status" | grep -q '"setupRequired":true'
curl -fsS "$BASE/docs" | grep -q '<main'
curl -fsS "$BASE/install.sh" | grep -q 'RC_ENROLL_TOKEN'

setup_headers="$(curl -sS -D - -o /dev/null "$BASE/setup/$TOKEN")"
printf '%s' "$setup_headers" | grep -Eq '^HTTP/[^ ]+ 30[2378]'
printf '%s' "$setup_headers" | tr -d '\r' | grep -qi '^location: /$'
printf '%s' "$setup_headers" | grep -qi '^set-cookie: '

test "$(docker exec "$NAME" id -u rc)" = "10001"
test "$(docker exec "$NAME" id -g rc)" = "10001"
docker exec "$NAME" sh -c 'test "$(stat -c %a /data)" = 700'
docker exec "$NAME" sh -c '
  for file in /data/rc-v2.sqlite3*; do
    test "$(stat -c %a "$file")" = 600
    test "$(stat -c %U:%G "$file")" = rc:rc
  done
'
docker exec "$NAME" sh -c 'test "$(stat -c %a /data/ssh_host_ed25519_key)" = 600'
docker exec "$NAME" sh -c 'ps -eo user=,args= | grep -Eq "^rc[[:space:]]+.*/rc-server$"'
docker exec "$NAME" sh -c 'ssh-keyscan -T 2 -p 2222 127.0.0.1 >/dev/null 2>&1'

HOST_KEY="$(docker exec "$NAME" sha256sum /data/ssh_host_ed25519_key | awk '{print $1}')"
docker restart "$NAME" >/dev/null
refresh_base
wait_for_health
test "$HOST_KEY" = "$(docker exec "$NAME" sha256sum /data/ssh_host_ed25519_key | awk '{print $1}')"
docker exec "$NAME" /usr/local/bin/rc-server --healthcheck

STATUS="$(docker inspect "$NAME" --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}')"
test "$STATUS" = "healthy" || test "$STATUS" = "starting"
echo "container smoke: ok"
