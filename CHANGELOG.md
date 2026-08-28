# Changelog

All notable RC changes are recorded here. Published tags are immutable.

## [0.18.1] - 2026-08-28

### Changed

- Running `rc` without a command now prints the complete top-level help instead of a short banner. Native, kernel, and active WebAssembly component commands are grouped by account, remote control, Node, OpenSSH, components, diagnostics, platform, kernel, and extension surfaces.
- Added top-level `rc help COMPONENT_COMMAND` dispatch so dynamically registered component commands have the same discoverable help flow as native commands.

### Fixed

- Made the first browser WebRTC attempt genuinely direct by excluding TURN URLs while retaining host and STUN candidates. TURN is now used only by a relay-only second attempt after direct WebRTC fails, rather than being allowed to win the first ICE race.
- Read the browser-selected ICE candidate pair from transport statistics before falling back to nominated-pair discovery, so the terminal reports the actual direct or relayed route.

### Validation

- Added browser coverage that advertises an unreachable TURN server and requires encrypted terminal control to succeed over a non-relay candidate pair.
- Exercised the same direct-first flow from a secure HTTPS origin with a virtual passkey, separately enrolled Node, RC Lock synchronization, and a non-relay UDP DataChannel.

## [0.18.0] - 2026-08-28

### Added

- Added the first dogfoodable WebAssembly component runtime: the native RC release installs `rc-kernel` plus package-manager, local/HTTP/GitHub/OCI source, and diagnostics components. The existing Node remains native while its product domains move behind WIT.
- Added component-provided `rc add`, `remove`, `install`, `list`, `outdated`, `update`, `doctor`, `logs`, and `report` commands. Unknown top-level commands are dispatched to active Wasm components without adding them to the native CLI parser.
- Added a portable core-component release bundle and independent native-kernel release assets so Wasm is built once rather than once per operating system.
- Added `process_input`, completing the focused MCP process harness: discover machines, start a process, read incremental output, write exact stdin/EOF, and cancel it.
- Added signed RC Lock device directories, realm-scoped peer identities, signed topology and service advertisements, replay-protected opaque envelopes, content-addressed state digests, and deterministic transitive route calculation to the inactive mesh substrate.
- Added signed, versioned peer capability announcements and deterministic bilateral negotiation of the highest compatible version and shared feature set. Capability claims describe available behavior but never grant execution authority.
- Added a SQLite schema v1-to-v2 migration that records the execution-retention policy and reconciles process rows that could not survive a server restart.

### Changed

- `rc update` now updates managed Wasm components. `rc upgrade` updates the native RC platform and refreshes its core component bundle while preserving locally managed component overrides.
- New installs bootstrap the component runtime automatically after installing the native `rc` binary.
- Replaced MCP's ambiguous `timeoutSeconds` and byte `offset` descriptors with `waitSeconds` and an absolute `cursor`; the old argument names remain accepted for rolling compatibility but are no longer advertised.
- Changed MCP process output to ordered `stdout`/`stderr` chunks with `nextCursor`, `outputPending`, and `truncatedBeforeCursor`. The server keeps a rolling 256 KiB of the newest output and returns at most 64 KiB per status call instead of freezing once the first buffer fills.
- Removed arbitrary `minLength`/`maxLength` constraints from MCP string schemas. RC now enforces the real 2 MiB JSON-RPC request limit, 1 MiB Node control-message limit, and 128 KiB decoded stdin chunk limit at their transport boundaries.
- Kept live MCP process state until exit, loss, or explicit cancellation. Capacity exhaustion rejects a new process instead of silently evicting an active one.
- Made hosted SSH/MCP process ownership derive from the process manager's relay identity and terminate hosted processes when their Node transport disappears.
- Distributed pinned Node Ed25519 and X25519 public keys through Owner-authorized RC Lock snapshots. Existing pre-mesh locks bootstrap only the local identity until a current signed device directory is accepted.

### Security

- Kept all MCP command, stdin, stdout, and stderr plaintext memory-only and grant-bound; `process_input` and `process_cancel` re-check the originating grant, current process state, and current Owner role before reaching a Node.
- Bound mesh replay identifiers to their source peer, filtered topology edges to trusted realm peers, and rejected duplicate/self neighbor advertisements.

### Compatibility

- The production browser/CLI control path remains direct/TURN WebRTC. RC 0.18 ships the reviewed authority, topology, and lifecycle substrate but does not yet enable peer listeners, automatic discovery, root tunnelling, offline enrollment forwarding, or artifact caching.

## [0.17.2] - 2026-08-28

### Fixed

- Restored the exact pre-rewrite landing page and structured documentation render output instead of approximating their HTML, navigation, copy, footer, catalog, or article layout.
- Made browser WebRTC retries prefer TURN relay candidates when TURN is available, retained direct/STUN fallback when it is not, and gave the DataChannel enough time to open on slower networks.
- Versioned every RC-owned JavaScript and stylesheet URL with a new browser revision so an already-open browser cannot reuse a still-fresh entry bundle from the previous deployment after navigation.
- Added a persistent-Chrome end-to-end test that creates a virtual passkey, enrolls a real RC Node, executes a command over the encrypted browser WebRTC DataChannel, validates output, logs out, and checks the signed-out landing and docs.

## [0.17.1] - 2026-08-28

### Fixed

- Restored the indexed public landing page at signed-out `/` while keeping `/login` as the explicit passkey sign-in route.
- Accepted browser same-origin fetch metadata for form submissions that intentionally omit `Origin` and `Referer`, so sign-out revokes the session and clears its cookie instead of returning `invalid request origin`.
- Kept usable WebRTC offers when ICE gathering reaches its deadline with candidates already present, increased the gathering window, and rejected only candidate-free offers across browser, CLI, Node, and server peers.

### Validation

- Exercised a disposable production-equivalent topology with a real headless Chrome virtual passkey, a separately enrolled RC Node, a direct browser WebRTC DataChannel, terminal command/output round-trip, session revocation, landing-page return, and signed-out route protection.

## [0.17.0] - 2026-08-27

### Added

- Added `rc-context`, a typed runtime context with service leases, reverse-order synchronous/asynchronous effect cleanup, dependency-driven component activation, realm inheritance/isolation, and rollback-safe component replacement.
- Added `rc-mesh`, the realm-isolated mesh substrate for bounded opaque envelopes, cost-ordered route-provider failover, coordinator-role policy, revocation-freshness leases, and transport-independent encrypted frames.
- Decoupled CLI control encryption from WebRTC through `EncryptedFrameTransport`; WebRTC remains the only active browser/CLI transport while authenticated QUIC routing is developed behind the same authority boundary.
- Added exact MCP output schemas and the grant-bound `process_cancel` tool for INT, TERM, or KILL requests against active MCP processes.
- Added deterministic RS256 WebAuthn assertion coverage alongside the existing real-authenticator ES256 tests.

### Changed

- Disabled durable execution history by default. RC retains process rows only while they are needed for authorization, at-most-once execution, and reconnect reconciliation, then removes them after broadcasting the live final event.
- Kept final process metadata in a bounded five-minute in-memory cache so API/detail clients can observe completion without creating durable history.
- Made process and presence events live-only by default. `RC_EXECUTION_HISTORY=metadata` explicitly enables bounded lifecycle metadata retention; command text and process streams are never persisted.
- Reworked Node connection cleanup around owned effect scopes so secure sinks and control sessions unwind in reverse acquisition order.
- Replaced Node-side OpenSSL/WebAuthn verification with ring-based ES256 and RS256 verification, removed release-only vendored OpenSSL, and reduced duplicate or unused HTTP/runtime dependencies.

### Fixed

- Completed and reconciled hosted SSH/MCP process rows on exit or disconnect instead of leaving stale active metadata.
- Removed completed rows from the default device process list while preserving live terminal completion state and avoiding post-exit resync errors.
- Rechecked current Owner authority before MCP execution or cancellation and kept execution-capable grants out of RC Lock after demotion.

## [0.16.2] - 2026-08-27

### Fixed

- Made macOS service restart verification fail explicitly when a separately running `rc run` process keeps the singleton lock, instead of reporting a successful update while launchd repeatedly exits.
- Suppressed noisy `launchctl print` errors while probing whether the per-user service is already loaded.

### Changed

- Cross-build the Intel macOS release archive on the standard Apple Silicon `macos-15` runner, eliminating the dedicated Intel runner bottleneck while preserving the exact `x86_64-apple-darwin` artifact.
- Build vendored OpenSSL only for release archives; normal development and CI builds use the platform OpenSSL, removing most of the cold workspace-check cost while keeping shipped binaries self-contained.

## [0.16.1] - 2026-08-27

### Fixed

- Added `curl` to the production runtime image so Coolify's configured HTTP health check can mark the Rust server healthy instead of rolling back to the v0.15 container.
- Added a CI runtime-image assertion for `curl` so the production health-check dependency cannot regress silently.
- Refreshed the lockfile from yanked `chacha20` 0.10.1 to 0.10.2 so RustSec audit remains clean.
- Fixed first-instance setup authorization so the hashed setup cookie is accepted instead of being hashed a second time and rejected.
- Redirected `/login` to first-instance setup when no user exists and replaced the terse `409 no passkeys registered` response with an actionable authentication error for direct API callers.
- Imported Node Ed25519 identities as raw base64url keys in the browser, preventing intermittent terminal handshake `atob` failures.
- Kept terminal client and transport errors visible across live resyncs with an explicit dismiss action.
- Added the standard copy affordance to one-time device enrollment commands.

## [0.16.0] - 2026-08-27

### Breaking

- Replaced the Bun/TypeScript server and Go agent with a Rust server, Node runtime, and unified `rc` executable.
- Introduced the versioned `rc-v2.sqlite3` schema. v0.15 `rc.db`, passkeys, authority records, and agent state are not imported; operators must perform fresh setup and re-enroll Nodes.
- Removed the hosted browser/CLI WebSocket control fallback. Human terminal control is WebRTC-only.

### Added

- Passkey setup, login, registration, step-up, account lifecycle, workspace roles, invites, device enrollment, revocation, process metadata, and activity in the Rust server.
- Direct encrypted WebRTC control for browser and CLI clients, including process lifecycle reconciliation and RC Lock authority.
- Signed Node HTTP, proof-of-possession API/CLI clients, replay protection, scoped authority grants, and deterministic crypto fixtures.
- OpenSSH compatibility through `rc ssh-proxy`, registered SSH keys, and an isolated container SSH gateway.
- MCP OAuth 2.0 authorization code flow with PKCE, scoped grants, revocation, and bounded command tools.
- Verified self-install/update archives for macOS and Linux on arm64 and amd64.
- launchd and systemd user-service integration.
- Strict Rust/TypeScript CI, production-image validation, and operator/developer/security/release runbooks.

### Fixed

- Preserved remote CLI argument boundaries with safe shell quoting.
- Persisted the originating server URL in self-hosted enrollment commands.
- Rejected legacy/unversioned databases with an actionable error instead of partially opening them.
- Enforced private data-directory and SQLite file permissions, including the v0.16 container database name.
- Rejected invalid passkey origins before creating a database and explained the DNS/localhost requirement.
- Replaced poisonable runtime mutexes and removed panic-prone serialization and process-pipe assumptions.
- Hardened SSH key parsing, relay framing, process failure persistence, and helper EOF handling.
- Pinned the container user to UID/GID 10001 and added boot/restart health, permission, and host-key persistence checks.
- Made unified CLI/Node release artifacts self-contained by statically vendoring the OpenSSL dependency required by upstream WebAuthn COSE verification; HTTP remains Rustls-only.
- Added semantic main landmarks across authentication, setup, CLI authorization, OAuth consent, and error pages; improved responsive smoke coverage, documentation routing, and singular access labels.
