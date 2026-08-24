# Relay

Securely connect and manage your devices remotely.

## Run

```sh
docker build -t relay .
docker run --rm -p 3000:3000 -v relay-data:/data relay
```

Open `http://localhost:3000` and create the first account.

The web client is TypeScript under `web/`. `server.ts` imports `web/index.html`; Bun's production build bundles the server plus browser TypeScript/CSS/assets and emits content-hashed frontend assets. No separate frontend build script is maintained.

The device-side program is **OhRats Relay Node** (`ohrats-relay`). Running it with no arguments prints help. `enroll TOKEN` performs one-time enrollment, `run` keeps the node connected in the foreground, `status` shows local/remote state, `update` replaces the node binary, `config` manages node defaults, and `uninstall` removes its server registration, local state, and installed binary. `devices` and `device delete ID` use a full-account API token from `RELAY_API_TOKEN` (or `--token`) for account-level device management. State and config live under `~/.config/ohrats-relay`. The installer exits after install/enrollment instead of becoming the long-running node.

The product model is `User → Workspace → Device → Process`. Devices belong directly to one workspace. There are no fleet or browser-session abstractions.

Passkeys are the primary sign-in method. Full-access personal API tokens live under `/api`; a token can be used as `Authorization: Bearer rly_…` or exchanged on the sign-in screen for a normal browser session.

The browser uses ordinary HTTP for snapshots and CRUD plus one authenticated WebSocket for live state and interactive process control. The device page lists process history; each process opens on its own PTY route. Every remote process gets cwd, terminal size, stdin, resize, signals, retained output, and explicit exit/lost state. Process starts are never replayed after transport failure. If the Relay Node stops, crashes, updates, or is removed, its PTY lifeline closes and active remote processes are terminated. Nodes can be updated or removed from the web UI.

