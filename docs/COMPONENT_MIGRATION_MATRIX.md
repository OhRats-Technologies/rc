# Component migration matrix

This matrix defines the ownership boundary required before the native
`rc-server` product implementation can be removed. A row is complete only when
the component contract, implementation, HTTP/CLI adapter, persistence import,
and production-shaped test all use the component path.

| Product surface | Component owner | Thin native/kernel mechanism | Completion gate |
| --- | --- | --- | --- |
| Public landing and documentation | `webui-shell` | Generic HTTP dispatch | Exact historical pages and immutable assets render from the component |
| Setup, login, logout, browser sessions, passkeys | `identity-http`, `identity-store`, keyed WebAuthn verifier | Generic HTTP dispatch, durable store, clock/random | Virtual-passkey setup, restart, login, step-up, credential lifecycle, and account teardown pass |
| Users and account policy | `identity-store`, account HTTP/UI component | Generic HTTP dispatch | Rename/delete and final-user setup transition preserve account invariants |
| Workspaces, roles, membership, invitations | `workspace-store` plus workspace HTTP/UI component | Generic HTTP dispatch | Personal workspace, Owner invariants, invitation single use, access pages, and forms pass |
| Devices, enrollments, presence, tombstones | `device-store` plus device HTTP/UI component | Signed Node ingress and generic HTTP dispatch | Enroll/retry/reconnect/revoke/offline tombstone and presence tests pass |
| RC Lock and workspace authority | authority component | Protected key/resource handles and signed Node ingress | Bootstrap, signed transitions, replay rejection, local enforcement, and session invalidation pass |
| Browser/CLI control signaling | control component | Signed Node ingress and transport provider services | Challenge/open/close and selected transport negotiation pass without native domain state |
| Process policy and hosted lifecycle | process-policy/process-manager components | OS process/PTY handles and encrypted transport resources | Browser, CLI, MCP, and SSH share one at-most-once process primitive |
| WebRTC, TURN, and other transports | transport components | Socket/timer/random capability adapters | Direct, STUN, TURN, reconnect, and provider replacement tests pass |
| Structured events and browser SSE | `events-store`, events HTTP component | Generic streamed HTTP response | Ordered cursors, gap recovery, retention, disconnect, and browser live updates pass |
| API signing credentials and request authentication | `api-credential-store`, API HTTP component | Generic HTTP dispatch | Proof-of-possession, replay, scope, expiry, revocation, and OpenAPI tests pass |
| MCP OAuth, grants, and five process tools | `mcp-store`, MCP HTTP component | Generic streamed HTTP and process/control service calls | PKCE, resource binding, rotation, tool schemas, process streaming, and cancellation pass |
| SSH public keys and forced-command policy | `ssh-policy-store` | Stock `sshd`, bounded HTTPS/WebSocket byte bridge, process service | Key lifecycle, immutable device routing, shell/SFTP/SCP/rsync, disconnect, and authorization pass |
| Diagnostics and error reporting | diagnostics components | Bounded metadata host sink | Local, UI, mesh, limits, and redaction tests pass |
| Package sources and component updates | package/source/updater components | HTTP/filesystem adapters and trusted directory reconciliation | Local/HTTP/GitHub/OCI, digest locks, rollback, and live replacement pass |
| Kernel upgrade and recovery | updater component | Atomic replace/re-exec and embedded recovery bundle | Published upgrade, failed activation rollback, and recovery repair pass |
| Canonical server assembly | locked canonical profile | Kernel startup and container supervision | Clean-volume install, production data import, shadow topology, backup/restore, and cutover pass |

The native kernel may retain only Wasmtime hosting, WIT linking, lifecycle and
resource supervision, durable substrate, generic OS adapters, HTTP/stream and
signed-Node ingress mechanisms, CLI dispatch, recovery, and atomic re-exec.
Product names, schemas, authorization rules, page copy, package policy, and
transport selection belong to components.
