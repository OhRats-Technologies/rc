# Relay

Remote device control plane for OhRats Technologies.

## Rules

- Keep the control plane small and observable.
- Treat disconnects, stale devices, and interrupted jobs as normal states.
- Never store agent private keys or plaintext auth/enrollment tokens server-side.
- Humans authenticate only with WebAuthn passkeys. User records contain only opaque identity, display name, and timestamps; there is no password or recovery flow.
- API tokens are for programmatic access. Passkey management requires an authenticated browser session.
- Shared visual primitives come from `https://assets.ohrats.party/latest/ohrats.css`.
- Product CSS must use existing `--or-*` tokens; do not invent a parallel design system.
- API routes are versioned under `/api/v1`.
- Persist server state under `/data` in production.

