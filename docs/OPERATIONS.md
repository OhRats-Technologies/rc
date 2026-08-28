# Operations

This runbook covers a single RC server instance backed by a persistent SQLite volume. Run only one writer instance against a database file; RC does not implement multi-primary SQLite replication.

## Production container

```sh
docker build -t rc:0.18.0 .
docker volume create rc-data

export RC_SETUP_TOKEN="$(openssl rand -hex 32)"
docker run -d --name rc --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -v rc-data:/data \
  -e PUBLIC_URL=https://rc.example \
  -e RC_SETUP_TOKEN="$RC_SETUP_TOKEN" \
  -e RC_TRUST_PROXY=1 \
  rc:0.18.0
```

Terminate TLS at a controlled reverse proxy and forward to port 3000. Preserve WebSocket upgrades for `/api/v1/ssh/tunnel`, disable response buffering for SSE, and use long idle timeouts for SSE and SSH connections. Normal WebRTC media/data does not traverse the reverse proxy.

Only the HTTP port normally needs external exposure. The SSH daemon and internal SSH bridge ports are container-local implementation details.

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `PORT` | `3000` | HTTP listener on all interfaces |
| `PUBLIC_URL` | `http://localhost:<PORT>` | Exact public origin used by WebAuthn, cookies, OAuth, and generated links |
| `DATA_DIR` | `./data-v2` | Server-owned data directory; container sets `/data` |
| `RC_DB_PATH` | `<DATA_DIR>/rc-v2.sqlite3` | Optional explicit SQLite path |
| `STATIC_DIR` | `./dist/assets` | Browser asset directory; container sets `/app/assets` |
| `RC_SETUP_TOKEN` | generated at startup | One-time first-account link secret; always set explicitly in production |
| `RC_TRUST_PROXY` | disabled | Set to `1` only behind a proxy that strips client-supplied forwarding headers |
| `RC_PUBLIC_SIGNUP` | disabled | Set to `1` together with both Turnstile keys |
| `RC_TURNSTILE_SITE_KEY` | unset | Cloudflare Turnstile browser key |
| `RC_TURNSTILE_SECRET_KEY` | unset | Cloudflare Turnstile server secret |
| `RC_CF_TURN_TOKEN_ID` | unset | Cloudflare TURN key ID |
| `RC_CF_TURN_API_TOKEN` | unset | Cloudflare TURN API token |
| `RC_SSH_DAEMON_PORT` | `2222` | Loopback OpenSSH daemon port |
| `RC_SSH_INTERNAL_PORT` | `3001` | Loopback SSH helper bridge port |
| `RC_MCP_ACCESS_TTL_MINUTES` | `15` | Positive MCP access-token lifetime |
| `RC_EXECUTION_HISTORY` | `none` | `none` removes completed process metadata and does not audit process events; `metadata` retains lifecycle metadata only |
| `RC_EXECUTION_HISTORY_TTL_HOURS` | `168` | Positive retention window used only in `metadata` mode |
| `RUST_LOG` | `rc_server=info,tower_http=info` | `tracing_subscriber` filter |

Changing `PUBLIC_URL` changes the WebAuthn relying-party origin. Existing passkeys may stop working when the hostname changes. Treat the external origin as durable configuration.

Passkeys require a DNS hostname. Use `http://localhost:<port>` for local development; an IP-address `PUBLIC_URL` is rejected. Production origins must use HTTPS.

## First account

1. Start the server with an explicit random `RC_SETUP_TOKEN`.
2. Open `https://rc.example/setup/<token>` within 15 minutes.
3. Create the first passkey-backed owner.
4. Remove the setup token from routine operator notes and shell history. Keeping it in the environment is harmless after the first account exists, but rotating/removing it reduces secret sprawl.

Opening `/` directly does not authorize first-account creation when `RC_SETUP_TOKEN` is configured.

## Health and observability

- `GET /healthz` returns a plain liveness response.
- `GET /api/v1/health` returns `{"ok":true}`.
- `GET /api/v1/status` reports setup state and version without exposing secrets.
- The container health check runs `rc-server --healthcheck` against the loopback HTTP listener.
- Structured request and application logs are emitted to stdout/stderr.
- On macOS, the per-user Node launch agent writes `~/.config/rc/node.log`.
- `rc status` shows local enrollment plus the remote Node record.
- `rc service status` delegates to launchd or systemd.

Alert on repeated restarts, SQLite errors, authentication rate-limit spikes, sustained Node disconnects, and failed release updates.

Execution history is private by default. Live process and presence events still reach authenticated SSE clients, but are not inserted into the audit table. Setting `RC_EXECUTION_HISTORY=metadata` retains only process ID, device, origin, terminal flag, attribution, timestamps, final state, exit code/signal, and bounded error text. Command text, cwd, stdin, stdout, stderr, and terminal transcripts are never persisted by this policy.

## Backup

SQLite runs in WAL mode. Use one of these consistent approaches:

1. Stop the RC container, copy `rc-v2.sqlite3`, `rc-v2.sqlite3-wal`, and `rc-v2.sqlite3-shm` if present, then restart it.
2. Use SQLite's online backup API from a trusted host with access to the volume.

Also preserve deployment configuration and the exact image/tag. The database does not contain Node private keys, so server backup alone cannot recreate an enrolled Node; back up Node state separately only when your endpoint-security policy permits storing those secrets.

Test restoration into an isolated origin before relying on a backup.

## Restore

1. Stop the target server.
2. Empty the target data volume.
3. Restore the database files with ownership for UID/GID `10001` in the container.
4. Start the same RC version that created the backup.
5. Verify `/api/v1/status`, passkey login, device presence, and a harmless command on a test Node.
6. Put the restored instance into service only after the state is known-good.

The entrypoint enforces mode `0700` on `/data` and mode `0600` on recognized RC database files.
The container runtime user is pinned to UID/GID `10001` so restored volume ownership is deterministic.

## Updating a deployment

Server:

1. Take and verify a database backup.
2. Build or pull the intended immutable tag.
3. Replace the container without changing its persistent volume or `PUBLIC_URL`.
4. Confirm health and version endpoints.
5. Exercise login, device presence, and one test process.

Nodes:

```sh
rc update       # managed WebAssembly components
rc upgrade      # native RC platform + core component bundle
rc status
```

`rc upgrade` requires GitHub SHA-256 asset digests, validates the native RC and kernel archives plus the core component graph, refuses native downgrades, and restarts an installed Node service when the platform changed. `rc update` resolves only the desired managed component set and leaves the native binaries untouched.

## Rollback

Server rollback requires a database backup created by the same server build and the matching image. The CLI updater does not perform downgrades; replacing a Node binary with an older artifact is an explicit incident-recovery action.

## Incident recovery

- **Lost owner passkey:** another owner can invite or promote a replacement. With no usable owner passkey, restore a known-good backup; there is no password bypass.
- **Compromised API/CLI/SSH credential:** revoke it from the account UI, resynchronize workspace authority, and rotate any copied secret.
- **Compromised Node state:** delete the device in RC, remove `~/.config/rc` and its service on the machine, then enroll it as a new device.
- **Stale enrollment:** `rc enroll` refuses to overwrite unverifiable state. Use `rc uninstall` or remove the device deliberately before re-enrolling.
- **Database corruption:** stop the server, preserve all files for analysis, restore the latest tested backup, and re-enroll only Nodes whose records cannot be recovered.
