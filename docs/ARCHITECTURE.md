# Architecture

RC separates coordination from execution. The server coordinates requests; the
Node remains authoritative for machine access, local authority, and process
execution.

## Target topology

```text
Browser / rc CLI
       │ HTTPS: pages, API, signaling, SSE
       ▼
  rc-kernel + locked server component graph
       │
       │ signed Node HTTP + signaling
       ▼
  rc-kernel + locked Node component graph

Browser / rc CLI ══ encrypted WebRTC DataChannel ══ Node
OpenSSH ── hosted SSH gateway ── Node process
MCP ── OAuth + JSON-RPC/HTTP ── hosted execution adapter ── Node registry
```

Migration is incomplete while native product code remains on a production
path. [`ROADMAP.md`](../ROADMAP.md) is the authoritative checklist.

## Transport and trust

| Surface | Transport | Authentication | Data path |
| --- | --- | --- | --- |
| Browser pages/API | HTTPS | Passkey-backed cookie | Server HTML/JSON |
| Presence/events | SSE | Cookie or signed client | Metadata only |
| Browser/CLI signaling | HTTPS | Browser/CLI grant | Signaling only |
| Browser/CLI process traffic | WebRTC DataChannel | Node-verified authority | End-to-end encrypted |
| Node bootstrap/status | HTTPS | Ed25519 Node proof | Control metadata |
| SSH | WebSocket + OpenSSH | Registered SSH key | Hosted byte relay |
| MCP | OAuth + JSON-RPC/HTTP | Scoped grant | Hosted transit; Node-owned state/journal |

The server is trusted for identity and authorization. It does not need normal
browser/CLI terminal plaintext after WebRTC connects. SSH and MCP are explicit
hosted-transit exceptions. MCP request correlation is bounded and ephemeral;
the Node owns execution state and output journals.

The Node owns its long-term identity, transport secret, process runtime, and RC
Lock. It validates local authority and execution permits before starting a
process.

## Control flow

1. The controller authenticates and requests a process.
2. The server validates role, grant, scope, freshness, and device selection.
3. Controller and Node exchange signaling through HTTPS.
4. Both sides derive an authenticated control key.
5. Process frames use sequence-bound AES-GCM over WebRTC.
6. The Node validates RC Lock and the process permit before execution.
7. Active lifecycle metadata is published to server state and SSE.

Ambiguous starts are never replayed. Reconnect reconciliation marks hosted
processes lost when the Node no longer owns them.

## Components and services

WIT defines cross-component contracts. Component descriptors declare provided
and required services. The kernel owns activation, dependency reconciliation,
resource limits, generation draining, and narrow OS adapters.

New calls use the current healthy provider generation. Existing pinned
resources may drain on the previous generation before deactivation.

Native adapters may own only mechanisms such as sockets, process handles,
protected key handles, storage handles, resource meters, and atomic kernel
replacement. Product policy belongs to components.

Components have no ambient filesystem, network, process, key, or environment
access unless a declared host interface grants it.

## Capability and mesh rules

Capability announcements describe behavior; they never grant authority.
Negotiation is deterministic:

1. Match the capability identifier.
2. Select the highest shared version.
3. Require locally mandatory features.
4. Use the shared feature intersection.
5. Apply local provider policy.

Peer and route changes do not weaken RC Lock or authorize replay. Relays carry
opaque encrypted frames and do not decide whether a destination may execute.
Tier-0 (`rc.ohrats.party`) never accepts another coordinator as upstream
authority.

State classes are distinct:

- **Authority state:** exact-parent, Owner-authorized transitions; no automatic
  conflict merge.
- **Signed facts:** immutable signed operations such as device and revocation
  facts; revocation is monotonic.
- **Topology:** TTL-bound presence, adjacency, routes, and cost; not durable
  audit history.

## Persistence

Component-owned durable state is stored through the typed storage boundary.
Completed execution history is disabled by default.
`RC_EXECUTION_HISTORY=metadata` retains bounded lifecycle metadata only;
commands, cwd, stdin, stdout, stderr, and terminal transcripts remain
non-persistent.

Node state defaults to `~/.config/rc` on Unix and
`%LOCALAPPDATA%\OhRats\RC\state` on Windows:

| File | Contents |
| --- | --- |
| `device.json` | Device identity and transport secret |
| `config.json` | Server URL and optional device name |
| `account.json` | CLI client identity and signing key |
| `lock.json` | RC Lock authority |
| `node.log` | Node service log where the per-user service adapter uses a file |

## Source ownership during migration

```text
kernel/       native Wasmtime host and OS adapters
wit/          contracts
components/   product components
profiles/     graph assemblies
crates/rc-*   transitional native product code
web/          transitional global browser source
```

The transitional native product crates and global browser tree are removed as
their final semantics move behind tested component contracts.

## Build boundaries

- Kernel implementation changes build native kernel targets only.
- Component implementation changes build that component only.
- WIT changes rebuild importing components.
- Profile/lock changes assemble artifacts without compiling code.
- Native Rust dynamic libraries are not a plugin boundary.

Maintained source files stay below 300 lines.
