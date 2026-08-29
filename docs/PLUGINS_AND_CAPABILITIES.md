# Plugins and capability negotiation

RC uses one composition model for local services and peer capabilities without treating code loading as authority.

## Service model

Each swappable feature has three roles:

- a **service definition** owns the interface and semantics;
- a **provider** implements the service;
- a **consumer** depends on the service instead of a concrete provider.

WIT interfaces define services. Component descriptors declare requirements and
providers; the kernel keeps the active provider registry and owns the resource
handles needed to invoke them. A registration belongs to one component
generation and is withdrawn before that generation's effects are recovered.

The transitional `rc-context` and `rc-mesh` crates are migration inputs, not the
end-state plugin ABI. Their product semantics must move into independently
built components; only generic lifecycle and opaque host mechanisms remain
native.

A transport consumer depends on the transport service. Transport providers register behind that service, and dropping a provider lease exposes the next eligible provider without changing execution authority.

## Runtime profiles

Exact profile manifests and lockfiles compose named roles:

| Profile | Components |
| --- | --- |
| `node` | enrollment, RC Lock, process runtime, transports, updater |
| `controller` | account authorization, device discovery, encrypted control |
| `server` | HTTP routes, persistence, WebAuthn, workspaces, rendezvous |
| `gateway` | OpenSSH bridge and hosted relay adapters |
| `mcp` | OAuth resource metadata and process tools |

Profiles select component artifacts by digest; changing a profile assembles a
new graph without compiling code. They do not fork identity, execution, or
authority implementations.

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

### Native host adapters

The native host owns Wasmtime lifecycle and narrow operating-system mechanisms:
process and PTY handles, sockets, protected key handles, storage handles,
resource meters, and atomic kernel replacement. Product policy is not an
adapter concern.

### WebAssembly

Official components own product behavior including process policy and WebRTC
attempt planning. Other extension points include event-driven automation,
inventory enrichment, notifications, and bounded metadata transforms.

The host exposes narrow interfaces, denies ambient filesystem/network/process access, meters resources, binds host calls to RC capabilities, and verifies signed manifests and content digests.

Components receive typed records and opaque resource handles. They do not
receive ambient filesystem, network, process, key, or environment access. The
process policy validates specifications, access, PTY choices, signals,
authorization and termination timing, and buffer/input limits; the host
performs the approved OS calls. The WebRTC policy
filters ICE servers, returns the ordered attempt plan and timeouts, and
classifies selected routes; browser and native adapters perform WebRTC mechanics.

### Supervised processes

Heavy or independently deployed providers may run as supervised processes using a versioned protocol over pipes or a local socket. This provides process-level crash and resource isolation without loading native dynamic libraries into RC.

### Browser modules

Browser extensions are signed modules associated with a host component manifest. They use explicit server routes and named UI slots and do not receive Rust services or secret state directly.

## Build boundaries

Component boundaries also define rebuild boundaries:

- browser-only component asset changes rebuild only their owning component;
- standalone WebAssembly components do not relink `rc`;
- a native kernel implementation change builds only native kernel targets;
- a WIT change rebuilds only components that import the changed package;
- a profile or lock change assembles artifacts without compiling code.

Native Rust dynamic libraries are not a plugin boundary because Rust's native ABI is unstable. RC uses static Rust, WebAssembly components, supervised processes, or browser modules depending on the trust boundary.

Optional components must be removable without preventing RC from starting.
Core identity, authority, update, and recovery paths do not depend on
third-party extensions. A failed replacement leaves the previous healthy
generation active; successful withdrawal first makes the provider unavailable
to new consumers, drains dependents and pinned resources, then runs cleanup.
