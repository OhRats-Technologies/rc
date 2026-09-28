# Account-wide MCP grants (v2)

Consent now names the account, permissions, and lifetime, and explicitly includes
current and future machines. It does not require machines to be online. Observe
uses current workspace membership; Terminal still requires current Owner access.
OAuth PKCE, rotating refresh tokens, short-lived resource-bound access tokens,
and the fresh passkey ceremony remain unchanged.

## Signed contract and compatibility

Version 1 retains its `deviceIds` list and has no `audience` field. An empty
legacy list grants access to no devices.
Version 2 requires `audience: "account"` and an empty `deviceIds` list. Unknown
versions and ambiguous combinations fail closed. Signatures bind the exact grant
bytes through SHA-256 and the domain `rc-mcp-grant-v2`, distinct from version 1.
The account identifier, scopes, lifetime, client, and grant ID remain signed.

The server evaluates current membership on every request. Each Node independently
checks the passkey-backed control authority, Owner role in its local workspace
snapshot, signature, grant hash in RC Lock, Terminal scope, audience, and expiry.
An account grant is not permission to bootstrap or overwrite an existing lock.
New Nodes use the existing enrollment/TOFU boundary; later changes still need an
existing Owner's signed transition. This does not introduce a bearer API key.

Existing connections stay version 1. Reconnect the MCP client and approve the new
account consent once to adopt version 2; later enrollments need no OAuth renewal.
Nodes must advertise `mcp-account-v2` before account-wide execution is attempted.
Older Nodes reject v2 and receive an actionable upgrade error. No fallback may
expand a v1 device list or bypass RC Lock during a rolling upgrade.

## Offline delivery

The authority sync endpoint validates owner-signed transitions before persisting
them in schema-4 `authority_deliveries`. Each row binds a device, previous hash,
generation, next hash, complete signed message, and control-proof expiration.
No private signing key or execution content is stored. The durable write happens
before sending or completing consent. A failed database/signature operation
still fails consent; an offline Node does not.

Hello and LockState acknowledgements trigger delivery only for the exact local
hash/generation reported by the Node. Acknowledged predecessors are pruned;
expired proofs are not delivered. Pending target states are included among the
parents a subsequent Owner signs, so a reconnect can advance through an
intermediate snapshot without the server forging a transition. The Node rejects
stale/replayed transitions and invalidates live control sessions after acceptance.

Revocation denies further OAuth access immediately. Offline lock removal is
queued through the same signed path. An expired signing proof or a concurrent
authority change can still require a fresh authorized sync; machines never
accept unsigned updates merely because they have reconnected.

## Validation

Native tests cover v1 scope retention, v2 future devices, signature-domain
separation, ambiguous versions, local demotion, and missing grant hashes.
Server tests cover current membership, Owner enforcement, explicit empty-account
consent, schema migration, and offline transitions across restart and acknowledgement.
Browser consent clearly discloses future machines and queues sync before redirect.
