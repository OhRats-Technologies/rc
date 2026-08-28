# Runtime context and mesh architecture

RC v0.17 introduces the runtime substrate for a self-healing peer mesh while preserving the existing execution-authority and end-to-end-encryption boundaries.

The design is influenced by [*A Programming Paradigm for Spatiotemporal Composability*](https://github.com/cordiverse/paper/blob/main/paper.pdf). RC adopts the paper's practical separation between:

- **revertible effects**, whose cleanup is retained and applied when a component deactivates; and
- **reactive coeffects**, represented in RC as typed service requirements that determine whether a component may be active.

RC does not turn arbitrary remote commands into reversible operations. A component can undo a socket binding, route advertisement, subscription, child-process handle, or temporary registration. It cannot generally undo external changes made by a remote command.

## Durable-state rule

The durable state of RC is limited to identity, authority, configuration, revocation, enrolled objects, and persistence explicitly enabled by an operator.

By default RC does not retain execution history:

| Execution data | Default behavior |
| --- | --- |
| Command, arguments, cwd | Never persisted |
| stdin/stdout/stderr and terminal transcript | Never persisted |
| Process row while running | Retained only for live authorization/reconciliation |
| Completed process row | Removed after the live completion event |
| Process activity event | Broadcast live, not inserted into the audit table |
| MCP output | Bounded in-memory buffer with a short lifecycle |
| Presence and transport-connectivity events | Broadcast live, not audited |

`RC_EXECUTION_HISTORY=metadata` opts into bounded lifecycle metadata. It still never stores command text or process streams. `RC_EXECUTION_HISTORY_TTL_HOURS` controls retention for completed metadata and defaults to seven days.

At-most-once execution remains mandatory. The active process row and Node-local process ID are protocol-safety state, not an execution log. An ambiguous start is queried or marked lost; it is never replayed.

## `rc-context`

`rc-context` is the typed lifecycle layer shared by the server and Node.

It provides:

- parent/child realm contexts;
- typed and optionally named services;
- service leases that revoke registrations on drop;
- effect scopes with reverse-order synchronous or asynchronous cleanup;
- components with declared requirements;
- dependency-driven activation and deactivation;
- component replacement with rollback to the previous healthy component if activation fails.

Child contexts inherit parent services, but sibling contexts cannot see one another's local services. The server uses child contexts as the foundation for workspace/realm isolation.

The Node uses an effect scope for connection-owned resources. Losing a transport unwinds the secure process sink and control session in reverse acquisition order rather than relying on unrelated shutdown branches to remain synchronized.

## `rc-mesh`

`rc-mesh` defines the transport-independent mesh substrate:

- realm, peer, service, and route identities;
- opaque mesh envelopes with expiry and hop limits;
- a stable route broker with cost ordering, provider leases, failover, and realm isolation;
- an encrypted-frame transport interface used by direct WebRTC control;
- a component that publishes the route broker through `rc-context`.

The controller encryption layer now targets `EncryptedFrameTransport`; WebRTC is one provider rather than being fused to authorization and frame encryption. This is the seam through which direct QUIC and multi-hop QUIC routes can be added without granting the transport execution authority.

The v0.17 release does **not** advertise an operational peer-to-peer network yet. Direct browser/CLI control remains WebRTC-only. Shipping the transport abstraction and broker first lets later QUIC work preserve the current security semantics and be reviewed independently from the lifecycle/persistence change.

## Target peer model

The planned peer model distinguishes roles instead of copying the hosted server into every endpoint:

```text
rc.ohrats.party
  Tier-0 authority + public rendezvous
            │
       signed state
            │
        rc peer B
       /         \
  rc peer A     rc peer C
```

Every installed `rc` may become a leaf, authenticated relay, artifact cache, and limited coordinator. It does not become a browser/OAuth/SQLite authority server.

`rc.ohrats.party` is Tier-0:

- it never accepts another RC server as upstream authority;
- it never imports a peer database as truth;
- it may accept independently verifiable signed facts;
- established peers may continue operating during a root outage subject to an authority-freshness policy.

## Transport and routing invariants

Future QUIC providers must preserve these invariants:

1. QUIC encryption is hop-by-hop; RC control encryption remains end to end.
2. A relay routes an opaque envelope and never decides whether the destination may execute it.
3. The destination Node verifies RC Lock, controller/grant authority, sequence, expiry, and replay state.
4. Routes are isolated by realm and bounded by expiry, envelope size, rate, and hop limit.
5. There is no unauthenticated or general-purpose Internet relay mode.
6. Route changes do not authorize replay of an ambiguous process start.

The stable route broker allows consumers to depend on a route capability rather than a particular peer. A direct provider may disappear and a relay provider may replace it without reconstructing the execution or authority component.

## State classes

Mesh state is split deliberately:

### Strict authority state

Owners, roles, passkey material, API keys, execution-capable MCP grants, and RC Lock generations use exact-parent, Owner-authorized transitions. Conflicting authority branches never merge automatically.

### Convergent signed facts

Device enrollment and revocation can eventually be represented as signed immutable operations. Revocation tombstones are monotonic and win over additions. Peers distribute facts; the Tier-0 authority validates them rather than importing remote rows.

### Ephemeral topology

Presence, observed addresses, route cost, peer adjacency, and root reachability are TTL-bound advertisements. They are not durable audit history and may converge eventually.

## Planned delivery sequence

The mesh rollout is intentionally staged:

1. **v0.17 foundation** — context runtime, effect ownership, realm isolation, route broker, encrypted transport abstraction, and private-by-default execution lifecycle.
2. **Authenticated one-hop QUIC** — Node identity-bound peer sessions and explicit/LAN bootstrap.
3. **Opaque multi-hop routing** — signed topology advertisements, bounded forwarding, route failover, and root-service routes.
4. **Offline authority distribution** — signed RC Lock transitions, revocation freshness leases, and deterministic reconciliation.
5. **Verified artifact caching and mesh enrollment** — content-addressed signed releases and Owner-signed enrollment permits transported through peers.

Each phase must retain compatibility tests for existing WebRTC control, RC Lock transitions, at-most-once process semantics, and release verification before becoming enabled by default.
