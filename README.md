# Relay

Securely connect and manage your devices remotely.

## Run

```sh
docker build -t relay .
docker run --rm -p 3000:3000 -v relay-data:/data relay
```

Open `http://localhost:3000` and create the first account.

The device-side program is **OhRats Relay Node** (`ohrats-relay`). Running it with no arguments prints help. `enroll TOKEN` performs one-time enrollment, `run` keeps the node connected in the foreground, `status` shows local/remote state, `config` manages node defaults, and `uninstall` removes its server registration, local state, and installed binary. State and config live under `~/.config/ohrats-relay`; legacy `~/.config/relay/device.json` is migrated automatically. The installer exits after install/enrollment instead of becoming the long-running node. Browser state updates use authenticated SSE, while node control uses the outbound agent WebSocket.

