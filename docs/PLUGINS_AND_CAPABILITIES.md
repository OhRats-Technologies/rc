# Plugins and capability negotiation

RC uses one composition model for local services and peer capabilities without treating code loading as authority.

## Service model

Each swappable feature has three roles:

- a **service definition** owns the interface and semantics;
- a **provider** implements the service;
- a **consumer** depends on the service instead of a concrete provider.

`rc-context::Component` owns activation, requirements, and cleanup. `rc-context::Broker` keeps preferred and fallback providers behind a stable handle. `rc-mesh` advertises signed peer services and routes opaque encrypted frames.

A transport consumer depends on the transport service. Transport providers register behind that service, and dropping a provider lease exposes the next eligible provider without changing execution authority.

## Runtime profiles

The unified `rc` executable is composed from named roles:

| Profile | Components |
| --- | --- |
| `node` | enrollment, RC Lock, process runtime, transports, updater |
| `controller` | account authorization, device discovery, encrypted control |
| `server` | HTTP routes, persistence, WebAuthn, workspaces, rendezvous |
| `gateway` | OpenSSH bridge and hosted relay adapters |
| `mcp` | OAuth resource metadata and process tools |

Profiles select components; they do not fork identity, execution, or authority implementations.

## Peer capabilities

A capability announcement contains:

- a namespaced identifier such as `rc.transport.webrtc`;
- supported protocol versions;
- a sorted set of feature tokens.

Announcements are part of signed, expiring mesh advertisements. They are realm-scoped, sequence-ordered, and accepted only from peers whose Ed25519 keys are pinned by the local RC Lock.

Negotiation is deterministic:

1. Match the capability identifier.
2. Select the highest version supported by both peers.
3. Require every locally mandatory feature to appear remotely.
4. Use the intersection of locally and remotely supported features.
5. Let local policy choose among eligible providers.

A capability describes behavior. It never grants process execution, weakens RC Lock, or makes a peer authoritative.

## Provider registry

```text
EncryptedFrameTransport
  ├─ WebRTC provider
  ├─ direct peer provider
  └─ relay provider
```

The registry is local. Peer advertisements determine compatibility with a destination; provider priority, route cost, authority freshness, and operator policy remain local decisions.

## Extension boundaries

### Native Rust

Security-sensitive and data-plane components are statically linked Rust:

- RC Lock and passkey verification;
- process execution and lifecycle reconciliation;
- encryption and transport framing;
- persistence;
- release verification and update;
- the HTTP authority server.

Static linking keeps these boundaries reviewable and avoids a runtime Rust ABI.

### WebAssembly

Portable automation and policy extensions use explicit WebAssembly Component Model interfaces. Suitable extension points include event-driven automation, policy scoring, route preference, inventory enrichment, notifications, and bounded metadata transforms.

The host exposes narrow interfaces, denies ambient filesystem/network/process access, meters resources, binds host calls to RC capabilities, and verifies signed manifests and content digests.

Passkeys, RC Lock, raw process control, cryptography, and transport sockets stay outside this extension boundary.

### Supervised processes

Heavy or independently deployed providers may run as supervised processes using a versioned protocol over pipes or a local socket. This provides process-level crash and resource isolation without loading native dynamic libraries into RC.

### Browser modules

Browser extensions are signed modules associated with a host component manifest. They use explicit server routes and named UI slots and do not receive Rust services or secret state directly.

## Build boundaries

Component boundaries also define rebuild boundaries:

- browser-only changes do not rebuild Rust;
- standalone WebAssembly components do not relink `rc`;
- first-party Rust providers rebuild their dependency cone;
- shared protocol and authority changes rebuild their consumers.

Native Rust dynamic libraries are not a plugin boundary because Rust's native ABI is unstable. RC uses static Rust, WebAssembly components, supervised processes, or browser modules depending on the trust boundary.

Optional components must be removable without preventing RC from starting. Core identity, authority, update, and recovery paths do not depend on third-party extensions.
