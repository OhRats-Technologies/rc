# Relay

Securely connect and manage your devices remotely.

## Run

```sh
docker build -t relay .
docker run --rm -p 3000:3000 -v relay-data:/data relay
```

Open `http://localhost:3000` and create the first account.

The device-side program is **OhRats Relay Node** (`ohrats-relay`). The dashboard provides enrollment commands; `ohrats-relay uninstall` removes its server registration, local state, and installed binary. An enrolled node can update by rerunning `curl -fsSL https://relay.ohrats.party/install.sh | sh` after stopping the old process. Browser state updates use authenticated SSE, while node control uses the outbound agent WebSocket.

