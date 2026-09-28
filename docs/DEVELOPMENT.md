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

### Windows

Install Git, Rust 1.98 (MSVC), Bun 1.4, Python 3, Strawberry Perl, and Visual Studio 2022
Build Tools with the **Desktop development with C++** workload and Windows SDK.
Rust uses the native MSVC linker; installing VS Code alone is insufficient.
Strawberry Perl must be on PATH to build the server's bundled OpenSSL.
The repository's `rust-toolchain.toml` selects Rust, rustfmt, and clippy.
Open a new PowerShell after installing tools so it inherits their updated PATH.

```powershell
bun install --frozen-lockfile
cargo fetch --locked
bun run build:client
cargo build --locked -p rc-cli -p rc-server
cargo run --locked -p rc-server
```

Run the release installer separately to install the normal background Node:

```powershell
& ./public/install.ps1
rc status
```

This preserves an existing enrollment and adds `rc` to PATH. See
[installation](INSTALL.md) for enrolling a new machine and repairing legacy
Task Scheduler permissions. Development server state is separate from the
installed Node's state. Docker Desktop is only needed for container validation.

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

Native compilation starts alongside portable component builds; native jobs wait
for artifacts from the same workflow run only when they reach conformance.
Browser fixture changes select browser/service coverage without rebuilding
unrelated workspace tests. Rust source changes retain the native test gates.

Linux smoke jobs share the `rc-kernel` dependency cache. Keep their component
caches separate: storing a kernel dependency tree inside every smoke-job cache
exhausts the repository cache quota and evicts the expensive Windows/macOS caches.

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
