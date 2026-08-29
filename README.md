# RC

RC is a remote-control system for macOS and Linux nodes. It supports browser,
CLI, API, MCP, and OpenSSH access.

The repository is in an active migration from native product crates to a small
Wasmtime kernel plus independently built WebAssembly components. Production
still uses parts of `crates/rc-server` and `web/`; [`ROADMAP.md`](ROADMAP.md) is
the authoritative migration checklist.

## Runtime model

- The server coordinates identity, authorization, presence, signaling, MCP,
  and SSH gateway traffic.
- The Node enforces RC Lock and process execution locally.
- Browser and CLI process traffic uses encrypted WebRTC DataChannels after
  HTTP signaling.
- SSH and MCP are hosted relay surfaces and have different trust boundaries.
- API and CLI automation credentials use Ed25519 proof-of-possession, not
  bearer tokens.

## Repository layout

| Path | Purpose |
| --- | --- |
| `kernel/` | Native Wasmtime host and narrow OS adapters |
| `wit/` | Cross-component contracts and worlds |
| `components/` | Independently built product components |
| `profiles/` | Declarative component graph assemblies |
| `crates/rc-*` | Transitional native product implementations |
| `web/` | Transitional global browser source |
| `public/install.sh` | Release installer |
| `docker/` | SSH gateway support files |

Native product crates and the global browser tree are deletion queues, not the
target ownership model.

## Run locally

Required tools:

- Rust 1.98
- Bun 1.4
- Docker for container validation

Install dependencies:

```sh
bun install --frozen-lockfile
cargo fetch --locked
```

Build browser assets and run the current server:

```sh
bun run build:client
cargo run -p rc-server
```

For a production-shaped local container:

```sh
docker build -t rc .
export RC_SETUP_TOKEN="$(openssl rand -hex 24)"
docker run --rm --name rc \
  -p 3000:3000 \
  -v rc-data:/data \
  -e PUBLIC_URL=http://localhost:3000 \
  -e RC_SETUP_TOKEN="$RC_SETUP_TOKEN" \
  rc
```

Open `http://localhost:3000/setup/<RC_SETUP_TOKEN>` to create the first
passkey-backed account. Use HTTPS and the exact external `PUBLIC_URL` outside
localhost.

Health endpoints:

```text
/healthz
/api/v1/health
```

## Node and CLI

Generate an enrollment command from **Devices → Enroll device**, or use the
installer directly:

```sh
curl -fsSL https://rc.example/install.sh \
  | sh -s -- ENROLLMENT_TOKEN https://rc.example
```

Common commands:

```sh
rc login --url https://rc.example
rc devices
rc status
rc run DEVICE -- printf '%s\n' 'hello world'
rc shell DEVICE
rc ssh-key add ~/.ssh/id_ed25519.pub
rc ssh-config >> ~/.ssh/config
rc update
rc upgrade
rc list
```

`rc update` changes managed components. `rc upgrade` changes the native
platform and core component bundle.

## Validation

Use the same entry points as CI:

```sh
sh scripts/check-version.sh
sh scripts/check-source-size.sh
python3 scripts/check-component-boundaries.py
python3 scripts/validate-components.py
python3 scripts/validate-profiles.py
python3 scripts/test-affected-units.py
python3 scripts/check-doc-links.py
cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets --locked
bun run typecheck
```

Component-only changes should use `scripts/check-component.sh <name>`. A WIT
change must rebuild every importing component.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Development](docs/DEVELOPMENT.md)
- [Installation internals](docs/INSTALL.md)
- [Operations](docs/OPERATIONS.md)
- [API signing](docs/API.md)
- [Releases](docs/RELEASES.md)
- [Security](SECURITY.md)
- [Migration roadmap](ROADMAP.md)
- [Acceptance baseline](CHECKLIST.md)
- [Release history](CHANGELOG.md)

Repository implementation rules are in `AGENTS.md`.

## License

`UNLICENSED`.
