# Development

## Toolchain

- Go 1.26.5 or newer for the native binaries (`cmd/rc`, `cmd/rc-server`)
- Go 1.27.1 when building components: every `components/*/go.mod` declares it
- `componentize-go` v0.5.3 for WebAssembly components
- Bun 1.4.4 for the typed browser client
- Docker for runtime-image validation
- A C toolchain: `pkg/kernel` links wasmtime-go through cgo
- macOS, Linux, or Windows for native Node/service integration

Install dependencies without changing lockfiles:

```sh
bun install --frozen-lockfile
go mod download
go install github.com/bytecodealliance/componentize-go@v0.5.3
```

`componentize-go` is installed into `$(go env GOPATH)/bin`; keep that directory on
`PATH`. Go 1.21+ downloads the component toolchain declared by each module's
`go` directive automatically unless `GOTOOLCHAIN=local`.

### Windows

Install Go 1.27.1, Bun 1.4.4, Python 3, and MinGW-w64 (`gcc` on `PATH`).
`pkg/kernel` uses cgo to link the prebuilt wasmtime archive, so the MSVC
toolchain alone is insufficient. Docker Desktop is only needed for container
validation.

```powershell
bun install --frozen-lockfile
go mod download
go install github.com/bytecodealliance/componentize-go@v0.5.3
bun run build:client
go build -o dist/rc.exe ./cmd/rc
go build -o dist/rc-server.exe ./cmd/rc-server
```

The release installers target the single Go `rc` archive and core profile; see
[installation](INSTALL.md). Development server state is separate from the
installed Node's state.

## Current local server

`cmd/rc-server` serves `/healthz`, immutable assets, and the product routes that
the Go runtime implements. Signed Node authentication, passkey sessions, and
SSE are still pending, so `/api/v1/node/enroll`, `/api/v1/devices*`,
`/api/v1/events`, and `/mcp` fail closed with HTTP 503 unless the development
registry is explicitly enabled:

```sh
bun run build:client
RC_DEV_INSECURE=1 RC_DEV_ENROLL_TOKEN="$(openssl rand -hex 16)" go run ./cmd/rc-server
```

The token is one-time and only exists in the in-memory development registry.
Never enable `RC_DEV_INSECURE` in a deployment.

## Validation

Static checks:

```sh
gofmt -l cmd pkg tests
go vet ./...
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

Native Go:

```sh
go build ./...
go test ./...
CGO_ENABLED=0 go build ./...   # portable kernel-free build
go test ./tests/integration/ -count=1 -v
```

The integration harness starts `cmd/rc-server` on a kernel-assigned port and
drives `cmd/rc` and `dist/components/*.wasm` against it. It skips component
tests when `componentize-go` is not installed.

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
scripts/build-component.sh components/COMPONENT
```

Each component owns its `go.mod` and generated WIT bindings. `componentize-go`
writes `dist/components/<artifact>.wasm`; the kernel never imports a component
module.

CI selects affected component, runtime, profile, browser, and image jobs through
`scripts/affected-units.py`. Linux component builds share the Go module cache;
keep the kernel build cache separate so one job's module downloads do not evict
the Windows/macOS caches.

## Runtime status

All first-party component bodies and native CLI commands are implemented. The
kernel still links unknown component imports as traps and does not invoke
exports, so component behavior is not executed end to end; the hosted passkey
session and WebRTC control plane are also pending. `docs/GO-MIGRATION-STATUS.md`
tracks the remaining work and which checks are gated on it.

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
