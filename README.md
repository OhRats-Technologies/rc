# RC

RC is a Rust remote-control platform for managing machines from a browser, CLI, OpenSSH, API client, or MCP-compatible agent. Human terminal traffic uses application-layer encrypted WebRTC DataChannels directly between the controller and RC Node; the server coordinates identity, authorization, presence, and ICE/TURN.

The repository contains the server, browser application, Node runtime, CLI, protocol, cryptography, installer, container image, and release automation.

## Components

| Component | Purpose |
| --- | --- |
| `rc-server` | Axum HTTP service, passkeys, workspaces, signaling, SSH gateway, MCP OAuth/RPC, SQLite persistence |
| `rc` | Account CLI, remote command/shell client, Node executable, updater, and service manager |
| `rc-node` | Enrollment, signed Node HTTP, WebRTC control, process runtime, RC Lock, state, and updates |
| `rc-api-client` | Proof-of-possession API client and control bootstrap |
| `rc-protocol` | Shared wire messages and authority structures |
| `rc-crypto` | Request signatures, key agreement, control encryption, and WebAuthn helpers |
| `rc-context` | Typed runtime services, revertible effect scopes, and dependency reconciliation |
| `rc-mesh` | Realm-isolated route broker and encrypted transport substrate |
| `web/` | Browser TypeScript and CSS bundled by Bun; Bun is not present in the runtime image |

The MCP Terminal surface intentionally exposes one process workflow: `machines_list` → `process_run` → `process_status`, with `process_input` for exact stdin/EOF and `process_cancel` for termination. Output is an ephemeral rolling in-memory stream addressed by cursor; RC does not persist MCP commands, input, or output.

## Run the server locally

The fastest production-shaped setup is Docker:

```sh
git clone git@github.com:OhRats-Technologies/rc.git
cd rc
docker build -t rc .

export RC_SETUP_TOKEN="$(openssl rand -hex 24)"
docker run --name rc --rm \
  -p 3000:3000 \
  -v rc-data:/data \
  -e PUBLIC_URL=http://localhost:3000 \
  -e RC_SETUP_TOKEN="$RC_SETUP_TOKEN" \
  rc
```

Open the one-time setup URL, not the bare home page:

```text
http://localhost:3000/setup/<RC_SETUP_TOKEN>
```

Create the first passkey-backed owner account. The setup authorization cookie lasts 15 minutes. For a non-local deployment, configure HTTPS and set `PUBLIC_URL` to the exact external origin before creating passkeys.

Health checks are available at `/healthz` and `/api/v1/health`.
The production image also declares a Docker health check backed by `rc-server --healthcheck`.

## Enroll a Node

In the browser, open **Devices → Enroll device**, choose an owned workspace, and generate the one-time install command. The command includes the server URL so self-hosted Nodes reconnect to the correct origin.

The equivalent installer interface is:

```sh
curl -fsSL https://rc.example/install.sh \
  | sh -s -- ENROLLMENT_TOKEN https://rc.example
```

The installer verifies and installs the matching native `rc` release, the platform kernel, and the portable core WebAssembly component bundle. It then enrolls the Node and installs a per-user launchd or systemd service when enrollment state exists.

Useful CLI commands:

```sh
rc login --url https://rc.example
rc devices
rc status
rc run DEVICE -- printf '%s\n' 'hello world'
rc shell DEVICE
rc ssh-key add ~/.ssh/id_ed25519.pub
rc ssh-config >> ~/.ssh/config
rc service status
rc update
rc upgrade
rc list
```

`rc update` updates managed components. `rc upgrade` updates the native RC platform and core bundle. Run `rc` with no command for grouped help, or `rc commands` to inspect commands provided by active components.

## Develop

Required tools are Rust 1.98 and Bun 1.4. Docker is optional but required for runtime-image validation.

```sh
bun install --frozen-lockfile
bun run typecheck
bun run build:client
cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
```

Start a development server with:

```sh
cp .env.example .env
bun run dev
```

The kernel keeps compiled Wasmtime artifacts in `cache/wasmtime` beside its
configured component directory. `RC_WASMTIME_CACHE_DIR` provides an isolated
override for tests. RC supplies this directory directly instead of loading the
global Wasmtime configuration; Wasmtime keys entries by its engine and compiler
configuration and cleans toward RC's 256 MiB / 4,096-file soft limits hourly.

The server logs a generated setup URL when `RC_SETUP_TOKEN` is unset. Never use that convenience behavior for a shared production log stream; set an explicit secret instead.

## Documentation

- [Architecture and data flows](docs/ARCHITECTURE.md)
- [Runtime context and mesh architecture](docs/CONTEXT_AND_MESH.md)
- [Deployment, backup, and recovery](docs/OPERATIONS.md)
- [Development and test matrix](docs/DEVELOPMENT.md)
- [Proof-of-possession HTTP API](docs/API.md)
- [Release process and rollback](docs/RELEASES.md)
- [Security model and reporting](SECURITY.md)
- [Release history](CHANGELOG.md)

`AGENTS.md` contains repository implementation invariants, and `CHECKLIST.md` is the engineering acceptance checklist.

## License

This repository is proprietary and unlicensed for redistribution (`UNLICENSED`).
