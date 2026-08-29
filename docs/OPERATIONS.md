# Operations

This runbook describes the current single-instance production server. The
server still contains transitional native product code; production cutover to
the canonical component graph is tracked in `ROADMAP.md`.

## Container

```sh
docker build -t rc .
docker volume create rc-data

export RC_SETUP_TOKEN="$(openssl rand -hex 32)"
docker run -d --name rc --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -v rc-data:/data \
  -e PUBLIC_URL=https://rc.example \
  -e RC_SETUP_TOKEN="$RC_SETUP_TOKEN" \
  -e RC_TRUST_PROXY=1 \
  rc
```

Terminate TLS at a trusted reverse proxy. Preserve WebSocket upgrades for the
SSH tunnel and disable response buffering for SSE. Browser/CLI WebRTC traffic
does not use the HTTP reverse proxy after signaling.

Run one server writer against a SQLite volume.

## Required configuration

| Variable | Purpose |
| --- | --- |
| `PUBLIC_URL` | Exact public origin for WebAuthn, cookies, OAuth, and links |
| `DATA_DIR` | Server data directory; container default `/data` |
| `RC_SETUP_TOKEN` | First-account setup secret |
| `RC_TRUST_PROXY` | Trust sanitized forwarding headers when set to `1` |
| `RC_PUBLIC_SIGNUP` | Enable managed public signup when set to `1` |
| `RC_TURNSTILE_SITE_KEY` / `RC_TURNSTILE_SECRET_KEY` | Public-signup Turnstile keys |
| `RC_CF_TURN_TOKEN_ID` / `RC_CF_TURN_API_TOKEN` | Backend-only Cloudflare TURN credentials |
| `RC_EXECUTION_HISTORY` | `none` or `metadata` |
| `RC_EXECUTION_HISTORY_TTL_HOURS` | Metadata retention window |

Keep long-lived TURN credentials server-side. Browser and Node clients receive
only short-lived ICE credentials.

## Health

```text
/healthz
/api/v1/health
/api/v1/status
```

The container health check uses `rc-server --healthcheck`.

## Backup

Back up the complete `/data` volume while preserving file ownership and modes.
The SQLite database must be captured consistently; use the application backup
path or stop the writer before copying raw database files.

Verify backups by restoring them into an isolated instance and checking:

1. health endpoints;
2. passkey login;
3. workspace/device state;
4. Node presence;
5. one harmless process.

## Restore

1. Stop the server.
2. Restore the matching database and data files.
3. Restore ownership for container UID/GID `10001`.
4. Start the same compatible server version.
5. Run the backup verification checks.

The entrypoint enforces mode `0700` on `/data` and mode `0600` on recognized
database files.

## Update

Server:

1. Verify a backup.
2. Build or pull the intended immutable image.
3. Replace the container without changing `/data` or `PUBLIC_URL`.
4. Verify health, login, Node presence, and one process.

Nodes:

```sh
rc update
rc upgrade
rc status
```

`rc update` changes managed components. `rc upgrade` changes the native
platform and core component bundle.

## Incident actions

- Lost owner passkey: use another Owner or restore a known-good backup.
- Compromised client credential: revoke it and refresh affected authority.
- Compromised Node state: delete the device, clear local state, and re-enroll.
- Database corruption: stop the server, preserve evidence, and restore a tested
  backup.

There is no password or recovery bypass for passkey authority.
