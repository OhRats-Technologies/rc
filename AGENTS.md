# Relay

Remote device control plane for OhRats Technologies.

## Rules

- Keep the control plane small and observable.
- Treat disconnects, stale devices, and interrupted processes as normal states.
- Never store agent private keys or plaintext auth/enrollment tokens server-side.
- Humans authenticate only with WebAuthn passkeys. User records contain only opaque identity, display name, and timestamps; there is no password or recovery flow.
- API tokens are for programmatic access. Passkey management requires an authenticated browser session.
- Shared visual primitives come from `https://assets.ohrats.party/current/ohrats.css`.
- Product CSS must use existing `--or-*` tokens; do not invent a parallel design system.
- Keep frontend source modular. `build.ts` fingerprints and bundles production JS/CSS/icons; hashed `/assets/*` are immutable, while the HTML shell revalidates.
- API routes are versioned under `/api/v1`.
- Persist server state under `/data` in production.
- Keep every maintained source file under 300 lines. Split by responsibility before a file reaches that size.
- Global navigation uses the persistent sidebar. Route-specific actions belong on their pages, not in global chrome or dialogs.
- Browser live state and interactive PTY control use one authenticated WebSocket. HTTP remains for snapshots and ordinary CRUD. Do not reintroduce timer polling.
- The device binary is branded `ohrats-relay` and presented as OhRats Relay Node.
- Keep the product model `User → Workspace → Device → Process`. Do not add fleets or browser shell-session wrappers without a concrete need.
- Every remote process owns a PTY and is at-most-once. If execution becomes ambiguous after disconnect/restart, mark it lost instead of replaying it.
- Relay Node shutdown, crash, and update must tear down all PTY process trees; never leave detached remote work behind.

