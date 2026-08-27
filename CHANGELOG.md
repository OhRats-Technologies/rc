# Changelog

All notable RC changes are recorded here. Published tags are immutable.

## [0.16.1] - 2026-08-27

### Fixed

- Added `curl` to the production runtime image so Coolify's configured HTTP health check can mark the Rust server healthy instead of rolling back to the v0.15 container.
- Added a CI runtime-image assertion for `curl` so the production health-check dependency cannot regress silently.
- Refreshed the lockfile from yanked `chacha20` 0.10.1 to 0.10.2 so RustSec audit remains clean.
- Fixed first-instance setup authorization so the hashed setup cookie is accepted instead of being hashed a second time and rejected.
- Redirected `/login` to first-instance setup when no user exists and replaced the terse `409 no passkeys registered` response with an actionable authentication error for direct API callers.

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
