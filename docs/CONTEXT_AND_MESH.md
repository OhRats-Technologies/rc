# Runtime context and mesh architecture

RC separates component lifecycle, peer routing, and execution authority. The mesh can change how encrypted frames reach a Node without changing who may execute a process.

The runtime model is influenced by [*A Programming Paradigm for Spatiotemporal Composability*](https://github.com/cordiverse/paper/blob/main/paper.pdf): components own revertible effects, while typed service requirements determine whether they can remain active.

Remote commands are not treated as reversible operations. Cleanup applies to resources RC owns directly, such as socket bindings, subscriptions, child-process handles, temporary registrations, and route advertisements.

## Durable state

Durable state is limited to identity, authority, configuration, revocation, enrolled objects, and persistence explicitly enabled by an operator.

| Execution data | Default behavior |
| --- | --- |
| Command, arguments, cwd | Never persisted |
| stdin/stdout/stderr and terminal transcript | Never persisted |
| Process row while running | Retained for live authorization and reconciliation |
| Completed process row | Removed after the live completion event |
| Process activity event | Broadcast live, not inserted into the audit table |
| MCP output | Bounded in-memory buffer |
| Presence and transport-connectivity events | Broadcast live, not audited |

`RC_EXECUTION_HISTORY=metadata` retains bounded lifecycle metadata without command text or streams. `RC_EXECUTION_HISTORY_TTL_HOURS` controls retention and defaults to seven days.

At-most-once execution is mandatory. An ambiguous process start is queried or marked lost; it is never replayed.

## Runtime context

`rc-context` provides:

- parent and child realm contexts;
- typed and optionally named services;
- service leases that revoke registrations on drop;
- effect scopes with reverse-order synchronous or asynchronous cleanup;
- components with declared requirements;
- dependency-driven activation and deactivation;
- component replacement with rollback when activation fails.

Child contexts inherit parent services. Sibling contexts cannot see one another's local services. The server uses child contexts for workspace isolation.

Node connections own an effect scope. When a transport ends, connection-owned resources are unwound in reverse acquisition order.

## Mesh substrate

`rc-mesh` provides:

- realm, peer, service, and route identities;
- peer identities derived from RC Lock-pinned Ed25519 keys;
- signed, expiring topology and service advertisements with monotonic sequence numbers;
- signed capability announcements and deterministic version/feature negotiation;
- deterministic route and service selection with trusted-peer filtering;
- opaque signed envelopes with expiry, replay protection, loop prevention, and hop limits;
- signed content-addressed authority, revocation, device-operation, and release digests;
- provider leases, cost ordering, failover, and realm isolation;
- the `EncryptedFrameTransport` interface used by encrypted control;
- a component that publishes the route broker through `rc-context`.

The control encryption layer targets `EncryptedFrameTransport`. Authorization and frame encryption do not depend on a particular transport provider.

RC Lock snapshots contain the device ID, Ed25519 identity public key, and X25519 transport public key for every trusted device in the workspace. A Node derives its mesh realm and trusted-peer directory only from its locally accepted RC Lock.

## Peer model

```text
Tier-0 authority / public rendezvous
              │
         signed state
              │
          rc peer B
         /         \
    rc peer A     rc peer C
```

Installed RC peers may act as leaves, authenticated relays, artifact caches, or limited coordinators. They do not become browser/OAuth/SQLite authority servers.

The Tier-0 authority:

- does not accept another RC server as upstream authority;
- does not import a peer database as truth;
- may accept independently verifiable signed facts;
- is not required to carry normal end-to-end encrypted terminal plaintext.

## Routing invariants

1. Transport encryption is hop-by-hop; RC control encryption remains end to end.
2. A relay routes an opaque envelope and never decides whether the destination may execute it.
3. The destination Node verifies RC Lock, controller/grant authority, sequence, expiry, and replay state.
4. Routes are isolated by realm and bounded by expiry, envelope size, rate, and hop limit.
5. There is no unauthenticated or general-purpose Internet relay mode.
6. Route changes never authorize replay of an ambiguous process start.

Consumers depend on route capabilities rather than individual peers. Provider loss may change the selected route without reconstructing execution or authority components.

Capability announcements are descriptive, not authoritative. Peers negotiate a shared capability version and feature set; local policy chooses a provider. The destination still performs its normal RC Lock and execution checks. See [Plugins and capability negotiation](PLUGINS_AND_CAPABILITIES.md).

## State classes

### Strict authority state

Owners, roles, passkey material, API keys, execution-capable MCP grants, and RC Lock generations use exact-parent, Owner-authorized transitions. Conflicting authority branches never merge automatically.

### Signed facts

Device and revocation facts are signed immutable operations. Revocation tombstones are monotonic and win over additions. Peers distribute facts; authority validation determines whether they are accepted.

### Ephemeral topology

Presence, observed addresses, route cost, peer adjacency, and root reachability are TTL-bound advertisements. They are not durable audit history.
