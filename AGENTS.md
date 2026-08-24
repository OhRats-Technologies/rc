# Relay

Remote device control plane for OhRats Technologies.

## Rules

- Keep the control plane small and observable.
- Treat disconnects, stale devices, and interrupted jobs as normal states.
- Never store agent private keys or plaintext auth/enrollment tokens server-side.
- Shared visual primitives come from `https://assets.ohrats.party/latest/ohrats.css`.
- Product CSS must use existing `--or-*` tokens; do not invent a parallel design system.
- API routes are versioned under `/api/v1`.
- Persist server state under `/data` in production.

