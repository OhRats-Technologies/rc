# Architecture

RC separates coordination from execution. The server decides who may request control, but the RC Node remains authoritative for machine access, local authority state, and process execution.

## Runtime topology

```text
Browser / rc CLI
       │ HTTPS: pages, JSON, signaling, SSE
       ▼
   rc-server ───────── SQLite
       │                    
       │ signed Node HTTP + signaling
       ▼
    RC Node

Browser / rc CLI ══ encrypted WebRTC DataChannel ══ RC Node

OpenSSH ── rc ssh-proxy ── public WebSocket ── SSH gateway ── RC Node process
MCP client ── OAuth 2.0 + JSON-RPC/HTTP ── rc-server ── bounded Node command relay
```

The public HTTP service is the rendezvous and policy plane. It does not carry normal browser or CLI terminal bytes after WebRTC is established. SSH and MCP are explicit exceptions because their protocols require a hosted bridge.

## Transport matrix

| Surface | Transport | Authentication | Payload handling |
| --- | --- | --- | --- |
| Browser pages/API | HTTPS | Passkey-backed browser cookie | Server-rendered HTML and JSON |
| Presence/activity | SSE | Browser or signed client identity | Metadata only |
| Browser/CLI control signaling | HTTPS | Browser/CLI authority grant | Offers, answers, and ICE candidates |
| Browser/CLI terminal | WebRTC DataChannel | Signed authority plus Node verification | Application-layer encrypted end to end |
| Node bootstrap/status | HTTPS | Ed25519-signed Node requests | Enrollment, presence, ICE, update metadata |
| SSH compatibility | WebSocket + local OpenSSH | Registered SSH key bound to a control client | Hosted byte relay to a Node process |
| MCP | OAuth 2.0 authorization code + PKCE, JSON-RPC/HTTP | Passkey-backed grant with machine and tool scopes | Bounded stdout/stderr retained by the service |

## Trust boundaries

### RC server

The server stores users, passkeys, browser sessions, workspaces, membership, devices, clients, grants, process metadata, and events. It validates request signatures, scopes, grants, replay nonces, and role requirements. It is trusted for identity and authorization decisions, but normal human terminal plaintext is not required at this boundary.

### RC Node

The Node owns its long-term Ed25519 identity seed, X25519 transport secret, process runtime, and RC Lock authority. It validates signed control grants and local authority transitions before opening a session. Removing a device creates a revocation tombstone so a stale local enrollment cannot silently reconnect.

### Controller

Browser and CLI controllers create local signing and transport material. CLI account and Node state files are written with owner-only permissions. API keys are proof-of-possession credentials; the server stores only their public keys.

## Control-session flow

1. A controller authenticates to the server and requests a process allocation.
2. The controller obtains a Node challenge and submits a signed, scoped control authorization.
3. The server verifies workspace role, client grant, request freshness, and requested device.
4. Controller and Node exchange WebRTC signaling through HTTPS.
5. Both sides derive session keys from authenticated key agreement.
6. Control messages and process streams use sequence-bound AES-GCM frames over the DataChannel.
7. The Node validates the process permit and RC Lock before executing.
8. Lifecycle metadata is reflected to SQLite and SSE; process output remains on the direct channel for browser/CLI control.

Reconnect reconciliation marks server-side `starting` or `running` processes as `lost` when the Node reports they no longer exist.

## Persistence

The Rust server uses SQLite WAL mode with foreign keys and a busy timeout. The default development database is `./data-v2/rc-v2.sqlite3`; the container default is `/data/rc-v2.sqlite3`. The data directory is mode `0700` and the main database is mode `0600` on Unix.

The schema is versioned through `PRAGMA user_version`. An unversioned non-empty database is rejected with a migration error rather than being interpreted as v0.16 data.

Node state defaults to `~/.config/rc`:

| File | Contents |
| --- | --- |
| `device.json` | Device ID, Node identity seed, transport secret |
| `config.json` | Server URL and optional device name |
| `account.json` | CLI client ID and signing seed |
| `lock.json` | Local RC Lock authority state |
| `node.log` | launchd service output on macOS |

## Source layout

```text
crates/rc-server      HTTP service and persistence
crates/rc-cli         rc executable and service integration
crates/rc-node        Node runtime, process manager, WebRTC, update
crates/rc-api-client  signed HTTP and control bootstrap client
crates/rc-protocol    serialized protocol structures
crates/rc-crypto      cryptographic primitives
web/client            browser TypeScript
web                   CSS and static source assets
public/install.sh     verified release installer
docker                SSH bridge support files
fixtures              cross-implementation protocol vectors
```

Production source files are kept below 300 lines and split by responsibility. Wire changes require protocol fixtures and tests before consumers are updated.

The unified `rc` release binary statically vendors OpenSSL only for the upstream Node-side WebAuthn/COSE verifier. HTTP TLS uses Rustls. This keeps macOS and Linux release artifacts independent of host OpenSSL packages and makes cross-compilation reproducible.
