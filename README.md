# Relay

Securely connect and manage your devices remotely.

## Run

```sh
docker build -t relay .
docker run --rm -p 3000:3000 -v relay-data:/data relay
```

Open `http://localhost:3000` and create the first account.

The device-side program is **OhRats Relay Node** (`ohrats-relay`). Running it with no arguments prints help. `enroll TOKEN` performs one-time enrollment, `run` keeps the node connected in the foreground, `status` shows local/remote state, `update` replaces the node binary, `config` manages node defaults, and `uninstall` removes its server registration, local state, and installed binary. State and config live under `~/.config/ohrats-relay`. The installer exits after install/enrollment instead of becoming the long-running node.

The product model is `User → Workspace → Device → Process`. Devices belong directly to one workspace. There are no fleet or browser-session abstractions.

The browser uses ordinary HTTP for snapshots and CRUD plus one authenticated WebSocket for live state and interactive process control. The device page lists process history; each process opens on its own PTY route. Every remote process gets cwd, terminal size, stdin, resize, signals, retained output, and explicit exit/lost state. Process starts are never replayed after transport failure. If the Relay Node stops, crashes, or updates, its PTY lifeline closes and active remote processes are terminated before the node reconnects. Nodes can be updated from the web UI or with `ohrats-relay update`.

