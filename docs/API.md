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
| Events | `GET /api/v1/events` (SSE metadata stream) |
| Tokens/SSH | Browser-only key management plus signed read/use operations |

Browser-session-only operations, including creating/revoking credentials and destructive account actions, require a fresh passkey step-up and cannot be performed with an API key.

## Errors

JSON failures use:

```json
{"error":"human-readable message"}
```

Common statuses are `400` malformed input, `401` absent/invalid/stale authentication, `403` missing scope or role, `404` inaccessible resource, `409` replay/conflict, `410` expired OAuth state, `413` body too large, and `429` rate limited. Do not retry `401`, `403`, or `409 replayed client request` with the same signature; generate a new timestamp and nonce after correcting the underlying condition.
