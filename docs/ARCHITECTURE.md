# Architecture

RC separates coordination from execution. The server decides who may request control, but the RC Node remains authoritative for machine access, local authority state, and process execution.

## Target runtime topology

```text
Browser / rc CLI
       │ HTTPS: pages, JSON, signaling, SSE
       ▼
  rc-kernel
       │
       ├── exact locked server component graph
       ├── narrow storage / HTTP / process / socket / key adapters
       └── /data (component-owned durable records)
       │
       │ signed Node HTTP + signaling
       ▼
  rc-kernel + exact locked Node component graph

Browser / rc CLI ══ encrypted WebRTC DataChannel ══ RC Node

OpenSSH ── rc ssh-proxy ── public WebSocket ── SSH gateway components ── RC Node process
MCP client ── OAuth 2.0 + JSON-RPC/HTTP ── server components ── bounded Node command relay
```

The migration is not complete while `crates/rc-server` or the native product
crates remain on a production path. The completion gates are recorded in the
[component migration matrix](COMPONENT_MIGRATION_MATRIX.md). Until cutover, the
legacy server preserves product behavior while each domain is moved behind WIT
contracts and its replaced native implementation is deleted.

The Node profile requires `ohrats:process-policy` and
`ohrats:transport-webrtc`. The native host has no product-policy fallback. A
control-open response carries the Node component's signed ICE attempt plan.
Browsers and the CLI execute that plan while native adapters own WebRTC socket
and process mechanics. New sessions use a replacement generation immediately;
established DataChannels continue until they close.

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
| MCP | OAuth 2.0 authorization code + PKCE, JSON-RPC/HTTP | Passkey-backed grant with explicit machine and tool scopes | Bounded, cursor-addressed in-memory stdin/stdout/stderr for the live MCP process lifecycle |

The MCP surface deliberately has one good path for each process operation:

| Tool | Contract |
| --- | --- |
| `machines_list` | Discover explicitly granted machine IDs and online state |
| `process_run` | Start one non-PTY shell process and optionally wait briefly for output or exit |
| `process_status` | Long-poll for ordered incremental stdout/stderr or completion using an absolute cursor |
| `process_input` | Write exact UTF-8 stdin bytes and optionally close stdin; RC never appends a newline |
| `process_cancel` | Request `INT`, `TERM`, or `KILL`, then use `process_status` to observe the final state |

`process_run.waitSeconds` and `process_status.waitSeconds` control only how long that RPC waits; they are not process runtime limits. Status responses preserve stdout/stderr labels and arrival order, return at most 64 KiB at once, and expose `nextCursor`, `outputPending`, and `truncatedBeforeCursor`. The server retains a rolling 256 KiB of the newest output in memory so a long-running process continues to make progress after old output expires. Live process control state is never silently evicted to make room; RC rejects a new MCP process when the active-process capacity is exhausted.

MCP string schemas intentionally do not declare arbitrary `minLength` or `maxLength` values. Semantic checks reject empty commands and invalid identifiers, while actual capacity is enforced at the layer that owns it: the HTTP JSON-RPC request body is limited to 2 MiB, a serialized Node control message to 1 MiB, and one decoded process-input chunk to 128 KiB. Each tool publishes an exact output schema. MCP Terminal grants are immutable, device-specific, Owner-approved, and included in each selected Node's RC Lock; newly enrolled machines require a new or replaced grant rather than inheriting ambient execution authority.

## Trust boundaries

### RC server

The server stores users, passkeys, browser sessions, workspaces, membership, devices, clients, grants, and durable security/product events. Active process metadata exists for authorization and reconciliation, but completed execution history and process events are not persisted by default. It validates request signatures, scopes, grants, replay nonces, and role requirements. It is trusted for identity and authorization decisions, but normal human terminal plaintext is not required at this boundary.

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
8. Active lifecycle metadata is reflected to SQLite and live SSE. Completion is broadcast and then removed by default; process output remains on the direct channel for browser/CLI control.

Reconnect reconciliation marks server-side `starting` or `running` processes as `lost` when the Node reports they no longer exist.

## Persistence

Completed execution history is disabled by default. `RC_EXECUTION_HISTORY=metadata` is an explicit opt-in for bounded lifecycle metadata; command text and streams remain non-persistent in every mode. Presence and transport-connectivity events are live-only. On startup, process rows that cannot still be running are reconciled as lost before the retention policy is applied.

## Component context and mesh substrate

WIT packages own service contracts. Component descriptors declare provided and
required services; the kernel reconciles activation, withdrawal, replacement,
and dependent shutdown. Registrations and host resources are generation-owned
effects. New consumers bind to the current healthy generation while resources
already pinned to an older generation drain before that generation is
deactivated.

Workspace isolation, RC-Lock-pinned peer keys, signed expiring topology and
capability announcements, replay-protected opaque envelopes, and route
selection are product semantics and therefore belong to components. Native
code may retain only opaque connection/resource handles and bounded transport
mechanics. The transitional `rc-context` and `rc-mesh` crates are not the
target composition ABI and must disappear after those semantics move behind
WIT. See [Runtime context and mesh architecture](CONTEXT_AND_MESH.md).

Local component services and remote peer negotiation share typed capability
identities without conflating code loading with authority. Installed code is
trusted by placement; execution authority still comes only from RC Lock and
operation-specific permits. See [Plugins and capability negotiation](PLUGINS_AND_CAPABILITIES.md).

The Rust server uses SQLite WAL mode with foreign keys and a busy timeout. The default development database is `./data-v2/rc-v2.sqlite3`; the container default is `/data/rc-v2.sqlite3`. The data directory is mode `0700` and the main database is mode `0600` on Unix.

The schema is versioned through `PRAGMA user_version`. An unrecognized non-empty database is rejected rather than interpreted as RC state.

Node state defaults to `~/.config/rc`:

| File | Contents |
| --- | --- |
| `device.json` | Device ID, Node identity seed, transport secret |
| `config.json` | Server URL and optional device name |
| `account.json` | CLI client ID and signing seed |
| `lock.json` | Local RC Lock authority state |
| `node.log` | launchd service output on macOS |

## Source layout during migration

```text
kernel                native Wasmtime host and narrow OS adapters
wit                   cross-component contracts and worlds
components            independently built product components
profiles              declarative component graph assemblies
crates/rc-*            transitional native product implementations
web                    transitional global browser source
public/install.sh     verified release installer
docker                SSH bridge support files
fixtures              cross-implementation protocol vectors
```

The `crates/rc-*` and global `web/` entries are deletion queues, not endorsed
end-state ownership. A native crate leaves only after its last product semantic
has a tested component owner and production-shaped adapter.

Production source files are kept below 300 lines and split by responsibility. Wire changes require protocol fixtures and tests before consumers are updated.

The unified `rc` release binary verifies Node-side ES256 and RS256 WebAuthn assertions with ring and uses Rustls for HTTP TLS. It has no OpenSSL runtime or build dependency, which keeps macOS and Linux release artifacts self-contained and makes cross-compilation reproducible.
