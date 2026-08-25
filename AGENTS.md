# RC

Remote device control plane for OhRats Technologies.

## Rules

- Keep the control plane small and observable.
- Treat disconnects, stale devices, and interrupted processes as normal states.
- Never store agent private keys or plaintext auth/enrollment tokens server-side.
- Passkeys are the human credential; there is no email/password or recovery flow. Human CLI login uses the passkey-backed browser authorization flow.
- API tokens are automation credentials only. They are hashed server-side, shown once, and explicitly scoped to read, execute, device management, and/or workspace management. Keep token management visible under the API route and do not offer tokens as browser-login substitutes. Token administration itself requires a human browser session.
- Shared visual primitives use content-fingerprinted immutable URLs under `https://assets.ohrats.party/assets/`.
- Follow cross-product UI guidance in `../handbook/design/ui.md`; when review feedback generalizes, promote it there/shared UI instead of keeping a one-off local fix.
- Product CSS must use existing `--or-*` tokens; do not invent a parallel design system.
- Keep browser source typed under `web/`. Bun remains the runtime and bundler; do not add a custom asset pipeline or untyped `public/js` tree.
- API routes are versioned under `/api/v1`.
- Persist server state under `/data` in production.
- Keep every maintained source file under 300 lines. Split by responsibility before a file reaches that size.
- Global navigation uses the persistent sidebar. Keep brand/collapse chrome pinned while the navigation tree scrolls. Workspace folders disclose device children; the folder click itself does not navigate. Disclosure height animates rather than snapping. Keep child lists bounded. Workspace and device rows share one composite row/context-menu treatment; device depth comes from inline-start indentation. Keep workspace enrollment/Actions/access/audit plus rename/delete lifecycle actions in the workspace ellipsis, and device rename/update/delete in the device ellipsis. Edit simple names directly in their labels and dismiss menus on outside interaction/Escape. Do not add workspace overview/detail pages that only duplicate sidebar functionality.
- Workspace creation is inline in the sidebar folder list. Enter creates; Escape/blur cancels. Do not reintroduce `/workspaces` or `/workspaces/new` HTML pages unless they gain materially distinct product functionality.
- Device presence is always visible in the hierarchy and uses shared semantic positive/negative state tokens. Long child labels may marquee only while hovered/focused and only when measured overflow exists; keep the row itself still and respect reduced motion.
- Device/workspace deletion is confirmed in the shared semantic sidebar dialog and then uses the authenticated DELETE API. Do not reintroduce dedicated delete-confirmation pages for these lightweight lifecycle actions.
- Keep the device page operationally minimal: platform + identity/presence, host + Node version, compact header rename/terminal/delete actions, and process history. Do not duplicate RC/server and Node versions when they are normally equal; surface version mismatch through the contextual update action. Do not reintroduce the custom command/cwd form in the device UI; explicit commands belong to Actions, API, or CLI.
- Retained PTY history must render through xterm's terminal parser even after completion so ANSI/OSC/cursor sequences are interpreted rather than exposed as text. Preserve the exact PTY stream when handing it to xterm, and keep a sanitized server-rendered text fallback for no-JavaScript use. Completed history must not open a live WebSocket or expose process controls.
- Standalone collection pages use compact header creation controls rather than permanent creation sections when the task is short. API key creation is a header-plus modal with a normal `/api/keys/new` form fallback; the one-time key is revealed immediately with the shared copy control. Actions use the same header-plus affordance but keep their full editor because creation is multi-field.
- A detail-page header may repeat the immediate identity/lifecycle actions that are useful while already viewing that object (for example device rename/delete) even when the persistent sidebar also exposes them contextually. Keep the action set compact and reuse the same underlying operation/dialog rather than creating a second workflow. Action and account deletion use the shared centered delete dialog. Native dialogs must remain explicitly centered despite the shared universal margin reset.
- Account display-name edits happen directly in the page title. Account deletion removes the account, deletes every workspace where that account is an Owner, leaves non-owned workspaces through membership cascade, preserves surviving historical attribution as `Deleted account`, and returns to protected setup when no human accounts remain.
- Top-level collection pages use one page title (`Devices`, `Actions`, `API`) without repeating the same noun as an eyebrow. Detail/workflow pages may use an eyebrow when it contributes object/context information such as `DEVICE`, `WORKSPACE / ACCESS`, or a workspace name plus `ACTION`.
- Device-list platform marks are based on operating-system platform, not CPU architecture. Keep macOS/Linux/Windows marks on the same normalized monochrome canvas and let shared CSS color them through `currentColor`; architecture remains textual metadata.
- Authenticated routes are ordinary server-rendered URLs. Do not require client-side routing for navigation. JavaScript may enhance individual pages but must not own primary page rendering.
- Prefer normal links, forms, POST/redirect flows, and server-rendered snapshots. JavaScript is reserved for capabilities that inherently require it or materially benefit from live enhancement.
- Keep authenticated pages visually flat: natural sections, typography, rows, and dividers. Do not turn every section into a bordered panel/card.
- Browser live state and interactive PTY control use one authenticated WebSocket. HTTP remains for snapshots and ordinary CRUD. Do not reintroduce timer polling.
- The device binary is branded `ohrats-rc` and presented as OhRats RC Node.
- Normal install/enrollment starts the Node as a per-user background service. `ohrats-rc run` is the foreground/debug path, not a required second terminal in the normal setup flow.
- Device removal is available from the web, `DELETE /api/v1/devices/:id`, and `ohrats-rc device delete`. Current nodes erase their local enrollment when removed remotely.
- Keep the product model `User → Workspace → Device → Process`. Do not add fleets or browser shell-session wrappers without a concrete need.
- Every remote process owns a PTY and is at-most-once. If execution becomes ambiguous after disconnect/restart, mark it lost instead of replaying it.
- RC Node shutdown, crash, and update must tear down all PTY process trees; never leave detached remote work behind.
- RC Node authentication uses a server-issued one-time challenge signed by the device Ed25519 identity. Never put reusable signatures or authentication secrets in WebSocket URLs.
- RC Node updates must verify the signed release manifest with the embedded OhRats Ed25519 release public key, verify the selected binary hash, and refuse signed downgrades. The release private key never belongs in the repository, runtime image, or RC server data.

