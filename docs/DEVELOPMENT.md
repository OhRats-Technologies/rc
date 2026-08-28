# Development

## Toolchain

- Rust 1.98, pinned by `rust-toolchain.toml`
- Bun 1.4 for dependency installation and browser bundling
- Docker for production-image validation
- macOS or Linux for the RC Node and service integration

Install dependencies without mutating the lockfile:

```sh
bun install --frozen-lockfile
cargo fetch --locked
```

## Local server

```sh
cp .env.example .env
bun run build:client
cargo run -p rc-server
```

When `RC_SETUP_TOKEN` is unset, the server logs a generated setup URL. The default development database is `./data-v2/rc-v2.sqlite3`.

The normal development and test profiles keep line-table debug information for useful panic locations while omitting full dependency debug data to reduce compile/link time and `target/` size. When a debugger needs full symbols, opt in explicitly:

```sh
cargo build --profile debugging
```

## Required validation

Run the same checks as CI before committing:

```sh
sh scripts/check-version.sh
sh scripts/check-source-size.sh
python3 scripts/check-doc-links.py
cargo audit
cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
bun run typecheck
bun run build:client
sh -n docker-entrypoint.sh docker/rc-ssh-authorized \
  docker/rc-ssh-bridge public/install.sh scripts/check-source-size.sh \
  scripts/check-version.sh scripts/smoke-container.sh
shellcheck docker-entrypoint.sh docker/rc-ssh-authorized docker/rc-ssh-bridge \
  public/install.sh scripts/check-source-size.sh scripts/check-version.sh \
  scripts/smoke-container.sh
actionlint -shellcheck shellcheck .github/workflows/*.yml
git diff --check
```

Validate the production image when Docker-related or server-runtime code changes:

```sh
docker build -t rc-ci .
sh scripts/smoke-container.sh rc-ci
```

## Test matrix

| Area | Primary coverage |
| --- | --- |
| API/Node request signatures and replay protection | `rc-crypto` vectors, `client_auth`, `enrollment_http` |
| Passkey setup/login/step-up/account lifecycle | `passkeys_http`, `account_delete` |
| Workspace/device/page authorization | `page_surfaces`, `control_http` |
| Node signaling and lifecycle | `node_webrtc`, `events_sse`, `relay_hubs` |
| Direct encrypted control | `rc-node/tests/control_webrtc.rs`, cross-language fixtures |
| RC Lock authority | `control_authority`, `lock_authority` |
| Process isolation and parity | `process_parity`, process-manager unit tests |
| Runtime context, effect cleanup, and mesh routing substrate | `rc-context/tests/runtime.rs`, `rc-mesh/tests/broker.rs`, `rc-mesh/tests/mesh.rs`, `mesh_authority` |
| Private-by-default process lifecycle | `execution_history`, `events_sse`, `node_webrtc` |
| MCP OAuth, five-tool descriptors, cursor output, stdin, cancellation, and isolation | `mcp_oauth`, `mcp_process_http`, `relay_hubs`, MCP descriptor/input unit tests |
| HTTP policy, CSP, cache, rate limits | `http_policy` |
| CLI parsing and command safety | `rc-cli` unit tests and release smoke tests |
| Browser assets and layout | TypeScript typecheck, Bun build, `scripts/webview-smoke.ts` |

Tests use temporary directories and ephemeral loopback ports. They must not depend on a developer's real `~/.config/rc`, browser account, or installed launch agent.

## Design rules

- Keep maintained Rust and TypeScript source files below 300 lines. Split by responsibility instead of compressing statements.
- Keep browser/server and Node/server protocol types in `rc-protocol`; do not duplicate wire literals across crates.
- Add or update deterministic fixtures for cryptographic or serialization changes.
- Treat Node execution as the final authority. Server allocation alone must never permit a process.
- Keep browser/CLI terminal data on encrypted WebRTC. Do not add a hosted fallback.
- Keep SSH and MCP relay behavior explicit and bounded.
- Never log tokens, private keys, passkey material, command plaintext, or terminal output.
- Preserve one-time/replay-resistant semantics for enrollment, nonces, authorization codes, and step-up tokens.
- Use `cargo fmt` formatting rather than dense one-line closures or transactions.

See `AGENTS.md` for the full implementation invariants and `CHECKLIST.md` for acceptance criteria.

## Browser smoke test

With a server running:

```sh
WEBVIEW_URL=http://127.0.0.1:3000/docs \
WEBVIEW_REQUIRE='main,h1' \
bun run smoke:webview
```

The script fails on browser console errors, navigation failures, missing assets/selectors, or horizontal overflow. Set `WEBVIEW_SCREENSHOT=/tmp/rc.png` to retain a screenshot.
Set `WEBVIEW_WIDTH` and `WEBVIEW_HEIGHT` to exercise responsive layouts; the defaults are `1440x900`.

Passkey ceremonies are tested with `webauthn-authenticator-rs` soft passkeys in Rust integration tests rather than mocked browser JSON.

## Protocol changes

1. Change types in `rc-protocol`.
2. Update canonical signing/encryption functions in `rc-crypto` when required.
3. Add deterministic fixtures under `fixtures/`.
4. Update Node, server, CLI, and browser consumers.
5. Add rejection tests for malformed, stale, replayed, unauthorized, and out-of-order messages.
6. Run the full workspace and browser validation matrix.

## Commit hygiene

Do not commit `target/`, `dist/`, `node_modules/`, databases, `.env`, Node state, account state, logs, or release archives. A completed change leaves `git diff --check` clean and updates documentation when behavior changes.
