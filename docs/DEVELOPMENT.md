# Development

## Toolchain

- Rust 1.98
- Bun 1.4
- Docker for runtime-image validation
- macOS, Linux, or Windows for native Node/service integration

Install dependencies without changing lockfiles:

```sh
bun install --frozen-lockfile
cargo fetch --locked
```

## Current local server

The component migration is not yet the production server path. Run the current
native server with:

```sh
cp .env.example .env
bun run build:client
cargo run -p rc-server
```

If `RC_SETUP_TOKEN` is unset, the development server logs a temporary setup
URL. Set an explicit token for shared environments.

## Validation

Static checks:

```sh
sh scripts/check-version.sh
sh scripts/check-source-size.sh
python3 scripts/check-component-boundaries.py
python3 scripts/check-diagnostics-capabilities.py
python3 scripts/validate-components.py
python3 scripts/validate-profiles.py
python3 scripts/test-affected-units.py
python3 scripts/check-doc-links.py
git diff --check
```

Native Rust:

```sh
cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets --locked
```

Kernel:

```sh
cargo fmt --manifest-path kernel/Cargo.toml --all -- --check
cargo clippy --manifest-path kernel/Cargo.toml --all-targets -- -D warnings
cargo test --manifest-path kernel/Cargo.toml --all-targets --locked
```

Browser:

```sh
bun run typecheck
bun run test:web
bun run build:client
```

Components:

```sh
scripts/check-component.sh COMPONENT
scripts/check-components.sh
```

Runtime smokes are under `scripts/smoke-*.sh`. CI selects affected component,
runtime, profile, browser, and image jobs through `scripts/affected-units.py`.

## Change rules

- Keep maintained source files below 300 lines.
- Put cross-component contracts in WIT.
- Preserve exact wire behavior with deterministic fixtures.
- Do not add ambient component capabilities.
- Delete replaced native behavior instead of adding compatibility layers.
- Update `ROADMAP.md` in the same commit when a migration gate changes.
- Keep unrelated working-tree changes intact.

See `AGENTS.md` for repository invariants and `CHECKLIST.md` for product
acceptance behavior.
