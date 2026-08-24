# Relay

Securely connect and manage your devices remotely.

## Run

```sh
docker build -t relay .
docker run --rm -p 3000:3000 -v relay-data:/data relay
```

Open `http://localhost:3000` and create the first account.

The web UI is server-rendered React/TSX under `web/server/` with small progressive-enhancement modules under `web/client/`. Bun builds those modules and product CSS into content-hashed `/assets/*` files; ordinary navigation and CRUD do not depend on a client-side router.

The device-side program and human CLI are both **OhRats Relay Node** (`ohrats-relay`). Running it with no arguments prints help. `enroll TOKEN` performs one-time device enrollment, automatically discarding a stale local identity when Relay confirms that device no longer exists; a still-valid enrollment is never silently replaced. `run` keeps the node connected in the foreground, `status` shows local/remote state, `update` replaces the node binary, `config` manages node defaults, and `uninstall` removes its server registration, local state, and installed binary. For human use, `login` opens Relay in the browser for passkey-backed CLI authorization, then `devices`, `shell DEVICE`, `run DEVICE -- CMD`, `actions`, and `action run` use that saved CLI session. `logout` revokes it. `RELAY_API_TOKEN` / `--token` remains an explicit automation override. State, config, and the protected CLI session live under `~/.config/ohrats-relay`. The installer exits after install/enrollment instead of becoming the long-running node.

The product model is `User → Workspace → Device → Process`, with saved `Action` definitions scoped to a workspace and executed through the same Process system. Devices belong directly to one workspace. There are no fleet or browser-session abstractions.

Passkeys are the human sign-in method. Browser sessions are created from passkeys; API tokens live under `/api` and are automation credentials only. CLI login uses a one-time browser approval flow and stores a separate expiring human CLI session rather than reusing an API token.

Workspaces have three roles: **Owner** manages access, workspace settings, Actions, and device lifecycle; **Operator** can open terminals and run Actions; **Viewer** can inspect inventory, Actions, and audit metadata without process output or execution. A workspace may have multiple owners and Relay will not allow its last owner to leave/demote themselves.

The browser uses ordinary HTTP for snapshots and CRUD plus one authenticated WebSocket for live state and interactive process control. The device page opens a normal login shell by default and keeps custom command/cwd as an advanced path. Browser terminals start from an 80×24 fallback, then xterm's fit addon resizes the PTY to the actual viewport and keeps it synchronized as the page changes size. Every remote process gets cwd, terminal size, stdin, resize, signals, retained output, ownership, and explicit exit/lost state. Operators control processes they created; owners can control any process in their workspace. Browser disconnects do not own process lifetime, and the Relay Node buffers/reconnects for a short grace window so a brief network loss or normal Relay control-plane restart does not kill active work. Device/Node termination, removal, or a disconnect beyond that grace still terminates the process tree.

Owners can save a successful command as an **Action**. Actions may require confirmation and can run on one or more selected devices; each device execution creates a normal Relay Process, so live output, history, permissions, and completion behavior are shared rather than implemented as a second job system.

