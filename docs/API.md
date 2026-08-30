# Proof-of-possession API

RC API keys are Ed25519 proof-of-possession credentials. A copied key ID or bearer token is insufficient: each request must be signed with the private key generated in the browser when the API key is created.

## Credential format

The browser shows the secret once:

```text
rcsk_<key-id>_<base64url-no-pad PKCS#8 Ed25519 private key DER>
```

Pass it to the CLI with `--token` or `RC_API_TOKEN`. The server stores the key ID, Ed25519 public key, scopes, lifetime, and usage metadata; it does not store the private key.

## Request headers

Every signed request supplies:

| Header | Value |
| --- | --- |
| `X-RC-Key-Id` | API/CLI client ID |
| `X-RC-Timestamp` | Current Unix time in whole seconds |
| `X-RC-Nonce` | Fresh base64url value, 16–128 characters |
| `X-RC-Signature` | Base64url-no-pad Ed25519 signature |

The timestamp must be within 60 seconds of server time. A successfully verified nonce is retained for 120 seconds; reuse returns `409 replayed client request`.

## Canonical payload

Sign the UTF-8 bytes of this exact string:

```text
rc-api-v1
<key-id>
<timestamp>
<nonce>
<uppercase HTTP method>
<path and raw query, beginning with />
<lowercase hex SHA-256 of the exact body bytes>
```

There is no final newline after the body digest. For a request without a body, hash the empty byte string. JSON must be serialized once and the exact resulting bytes both signed and sent. The request URI includes the raw query string in transmitted order but excludes scheme, authority, and fragment.

The canonical implementation is `rc_crypto::api_payload`; `rc_api_client::ApiClient` applies it automatically.

```rust
use rc_api_client::{ApiClient, Credential};

let credential = Credential::parse(&std::env::var("RC_API_TOKEN")?)?;
let client = ApiClient::new("https://rc.example", credential)?;
let devices = client.devices().await?;
# Ok::<(), Box<dyn std::error::Error>>(())
```

## Scopes

| Scope | Allows |
| --- | --- |
| `read` | Account-visible workspace, device, process, and activity reads |
| `execute` | Process allocation and direct control authorization |
| `manage-devices` | Enrollment creation, device rename/removal, and related actions |
| `manage-workspaces` | Workspace creation, membership, invites, roles, and deletion |

Routes enforce the narrowest relevant scope in addition to workspace roles. A key cannot grant access outside its owning user's memberships.

## Endpoint families

| Family | Representative routes |
| --- | --- |
| Identity/workspaces | `GET /api/v1/me`, `GET/POST /api/v1/workspaces`, workspace detail/access/activity routes |
| Devices/processes | `GET /api/v1/devices`, device detail, process allocation/detail/signal routes |
| Control | challenge, authorization, open, WebRTC offer/ICE, and close routes under `/api/v1/control` |
| Schedule authority | Owner-only permit metadata under `/api/v1/workspaces/:id/schedule-grants/:scheduleId` |
| Events | `GET /api/v1/events` (SSE metadata stream) |
| Tokens/SSH | Browser-only key management plus signed read/use operations |

Browser-session-only operations, including creating/revoking credentials and destructive account actions, require a fresh passkey step-up and cannot be performed with an API key.

Schedule permit routes store only schedule ID, device, Owner, immutable execution-spec hash,
maximum runtime, and expiry. Creating or replacing a permit requires a passkey-backed control
authorization issued within five minutes. The resulting authority snapshot still requires the
normal Owner control-key transition signature before a Node accepts it. Full cron, timezone,
argv/script, cwd, and environment definitions travel only over encrypted control and persist only
on the Node. Revocation requires a current passkey-backed control identity but no new ceremony.

## Process completion visibility

Completed execution history is not durable by default. While a process is starting or running, its row is available for authorization and reconciliation. After completion/loss, the server keeps the final metadata in a bounded in-memory cache for approximately five minutes so an API client can observe the terminal state, then removes it. The cache disappears on server restart and is never included in device process-history lists.

Deployments that explicitly set `RC_EXECUTION_HISTORY=metadata` retain completed lifecycle metadata for the configured TTL. Neither mode stores command text, cwd, stdin, stdout, stderr, or terminal transcripts.

## Remote MCP execution

Remote MCP keeps the focused `process_run`, `process_status`, `process_input`,
and `process_cancel` surface. `process_run` requires exactly one of `argv` or
`command`. `argv[0]` is executed directly with exact argument boundaries.
`command` is shell source; `shell: "rc"` selects portable RC Shell and
`shell: "system"` selects native target-machine shell semantics. Environment
defaults to inherited and supports explicit set/unset changes or a clean base.

`waitSeconds` only controls how long the call observes the new execution;
`maxRuntimeSeconds` controls its lifetime. A managed execution therefore
continues after its initiating RPC returns. `process_status` reads the
Node-owned bounded journal using an absolute cursor, reports truncation, and
preserves stdout/stderr boundaries. Each returned chunk carries its absolute
`cursor` and declares `encoding` as
`text` for valid UTF-8 or `base64` for arbitrary bytes and carries its value in
`data`; RC never silently replacement-decodes output. `process_input` writes exact bytes without
an implicit newline. Cancellation names semantic `INT`, `TERM`, or `KILL`
operations rather than Unix signal numbers.

`process_status`, `process_input`, and `process_cancel` require the original
`deviceId` together with `processId`. That non-sensitive routing input lets a
new hosted RC instance reach Node-owned state after a server restart. The Node
still requires the originating MCP grant ID on every operation; another grant
cannot access the execution merely because it knows both IDs.

MCP command and output plaintext may transit bounded hosted memory while an RPC
is serviced, but is not stored in SQLite, logs, traces, diagnostics, or process
history. Each operation revalidates the grant, selected device, scope,
principal, RC Lock, and process ownership; possession of a process ID is not
authority.

## Errors

JSON failures use:

```json
{"error":"human-readable message"}
```

Common statuses are `400` malformed input, `401` absent/invalid/stale authentication, `403` missing scope or role, `404` inaccessible resource, `409` replay/conflict, `410` expired OAuth state, `413` body too large, and `429` rate limited. Do not retry `401`, `403`, or `409 replayed client request` with the same signature; generate a new timestamp and nonce after correcting the underlying condition.
