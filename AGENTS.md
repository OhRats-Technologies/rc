# Relay

Remote device control plane for OhRats Technologies.

## Rules

- Keep the control plane small and observable.
- Treat disconnects, stale devices, and interrupted processes as normal states.
- Never store agent private keys or plaintext auth/enrollment tokens server-side.
- Passkeys are the primary human credential. Full-access API tokens may also exchange for a browser session; there is no email/password or recovery flow.
- API tokens are hashed server-side, shown once, and grant the same account authority as their owner. Keep token management visible under the API route.
- Shared visual primitives come from stable root URLs on `https://assets.ohrats.party/`.
- Follow cross-product UI guidance in `../handbook/design/ui.md`; when review feedback generalizes, promote it there/shared UI instead of keeping a one-off local fix.
- Product CSS must use existing `--or-*` tokens; do not invent a parallel design system.
- Keep browser source typed under `web/`. Bun remains the runtime and bundler; do not add a custom asset pipeline or untyped `public/js` tree.
- API routes are versioned under `/api/v1`.
- Persist server state under `/data` in production.
- Keep every maintained source file under 300 lines. Split by responsibility before a file reaches that size.
- Global navigation uses the persistent sidebar. Route-specific actions belong on their pages, not in global chrome or dialogs.
- Authenticated routes are ordinary server-rendered URLs. Do not require client-side routing for navigation. JavaScript may enhance individual pages but must not own primary page rendering.
- Prefer normal links, forms, POST/redirect flows, and server-rendered snapshots. JavaScript is reserved for capabilities that inherently require it or materially benefit from live enhancement.
- Keep authenticated pages visually flat: natural sections, typography, rows, and dividers. Do not turn every section into a bordered panel/card.
- Browser live state and interactive PTY control use one authenticated WebSocket. HTTP remains for snapshots and ordinary CRUD. Do not reintroduce timer polling.
- The device binary is branded `ohrats-relay` and presented as OhRats Relay Node.
- Device removal is available from the web, `DELETE /api/v1/devices/:id`, and `ohrats-relay device delete`. Current nodes erase their local enrollment when removed remotely.
- Keep the product model `User → Workspace → Device → Process`. Do not add fleets or browser shell-session wrappers without a concrete need.
- Every remote process owns a PTY and is at-most-once. If execution becomes ambiguous after disconnect/restart, mark it lost instead of replaying it.
- Relay Node shutdown, crash, and update must tear down all PTY process trees; never leave detached remote work behind.

