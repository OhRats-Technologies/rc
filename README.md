# OhRats RC

Remote control for your machines.

## Run

```sh
docker build -t rc .
docker run --rm -p 3000:3000 -v rc-data:/data rc
```

Open `http://localhost:3000` and create the first account.

The web UI is server-rendered React/TSX under `web/server/` with small progressive-enhancement modules under `web/client/`. Bun builds those modules and product CSS into content-hashed `/assets/*` files; ordinary navigation and CRUD do not depend on a client-side router.

The device-side program and human CLI are both **OhRats RC Node** (`ohrats-rc`). Running it with no arguments prints help. `enroll TOKEN` performs one-time device enrollment, automatically discarding a stale local identity when RC confirms that device no longer exists; a still-valid enrollment is never silently replaced. The installer registers an enrolled node as a per-user background service (LaunchAgent on macOS, user systemd service on Linux where available), so normal enrollment does not require keeping `ohrats-rc run` open in another terminal. `service` manages that background node; `run` remains the foreground/debug path. `status` shows local/remote state, `update` replaces the node binary, `config` manages node defaults, and `uninstall` stops/removes the service, server registration, local state, and installed binary. For human use, `login` opens RC in the browser for passkey-backed CLI authorization, then `devices`, `shell DEVICE`, `run DEVICE -- CMD`, `actions`, and `action run` use that saved CLI session. `logout` revokes it. `RC_API_TOKEN` / `--token` remains an explicit automation override. State, config, service logs, and the protected CLI session live under `~/.config/ohrats-rc`.

The product model is `User → Workspace → Device → Process`, with saved `Action` definitions scoped to a workspace and executed through the same Process system. Devices belong directly to one workspace. There are no fleet or browser-session abstractions.

Passkeys are the human sign-in method. Browser sessions are created from passkeys; API keys live under `/api` and are scoped automation credentials only (`read`, `execute`, `manage-devices`, `manage-workspaces`). Token administration requires a human browser session. The API page keeps only the active-key list and docs access in its normal layout: a header `+` opens a short name/permissions dialog, then reveals the new key once with the shared copy control. A normal `/api/keys/new` form preserves the same creation flow without JavaScript. CLI login uses a one-time browser approval flow and stores a separate expiring human CLI session rather than reusing an API key.

Workspaces have three roles: **Owner** manages access, workspace settings, Actions, and device lifecycle; **Operator** can open terminals and run Actions; **Viewer** can inspect inventory, Actions, and audit metadata without process output or execution. A workspace may have multiple owners and RC will not allow its last owner to leave/demote themselves.

The persistent sidebar is also the workspace control surface. Workspace folders only expand/collapse their device children; the ellipsis menu owns enrollment, Actions, access management, audit history, rename, and deletion. New workspaces are created inline from the sidebar `+`, so there is no separate workspace overview/detail/new page. Device rows use the same treatment with live red/green presence and contextual rename/update/delete actions.

Account names can be edited directly in the account page title. Account deletion removes credentials and memberships, deletes every workspace the account owns, leaves workspaces it does not own, preserves retained attribution in surviving workspaces under `Deleted account`, and returns RC to protected setup when the final human account is removed.

The browser uses ordinary HTTP for snapshots and CRUD plus one authenticated WebSocket for live state and interactive process control. The device page keeps the common path intentionally small: platform/device identity, host, installed Node version, compact rename/terminal/delete header actions, and process history. Explicit commands/cwd are handled by Actions, API, or CLI rather than a second advanced launcher in the device UI. Browser terminals start from an 80×24 fallback, then xterm's fit addon resizes the PTY to the actual viewport and keeps it synchronized as the page changes size. Completed process history is also rendered through xterm in read-only mode so retained ANSI/OSC terminal output is interpreted correctly; the server-rendered fallback strips control sequences for no-JavaScript clients. Every remote process gets cwd, terminal size, stdin, resize, signals, retained output, ownership, and explicit exit/lost state. Operators control processes they created; owners can control any process in their workspace. Browser disconnects do not own process lifetime, and the RC Node buffers/reconnects for a short grace window so a brief network loss or normal RC control-plane restart does not kill active work. Device/Node termination, removal, or a disconnect beyond that grace still terminates the process tree.

Node authentication is replay-resistant: the server issues a short-lived one-time challenge and the Node signs the challenge, method, and path with its local Ed25519 identity before WebSocket/status/unregister requests. Hosted-service hardening also applies application rate limits, restrictive browser security headers, configurable account/device/process quotas, shorter session/invite/enrollment lifetimes, and a non-root production container.

Node updates are release-signed independently of the RC runtime. The Node embeds only the OhRats Ed25519 release public key, verifies the signed `release.json` manifest, checks the selected artifact SHA-256 and reported version, and refuses signed downgrades before replacing itself. The release private key is kept outside the repository and runtime infrastructure.

Owners can save a successful command as an **Action**. Actions may require confirmation and can run on one or more selected devices; each device execution creates a normal RC Process, so live output, history, permissions, and completion behavior are shared rather than implemented as a second job system.

