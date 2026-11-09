# Go migration status

The source tree is Go. All first-party component export bodies, native CLI
commands, release verification, and doctor/upgrade paths are implemented. The
remaining work is the runtime that links components and the hosted account
control plane.

## Implemented

| Area | State |
|---|---|
| Components | All 60 `components/*` modules export real implementations; `panic("not implemented")` and generated TODO stubs are gone |
| Kernel loading | Descriptor-by-filename, digesting, cheap fingerprinting, reload, and failure isolation |
| Kernel host | cgo wrapper over the vendored wasmtime C component API; linker instances/functions, WASI, plugin host callbacks, HTTP client, keys custody, net byte-stream host with endpoint allowlists, SQLite-backed durable storage, process/environment/clock/filesystem hosts, policy/diagnostics stubs, nested interface calls, and real fixture execution |
| Component commands | Unknown `rc <command>` invocations load installed components, read their descriptors, activate the provider, and invoke the export; `rc commands` lists them |
| CLI | `version`, `status`, `devices`, `config`, `enroll`, `run`, `service`, `login`, `logout`, `logs`, `ps`, `shell`, `doctor`, `update`, `upgrade`, `repair`, `crypto-check`, `policy-check` |
| Presence | Signed `GET /api/v1/node/self` and `POST /api/v1/node/heartbeat`, replay window, nonce cache, online/offline events |
| Public web | Server-rendered landing/docs pages and token-gated setup/login/signup pages; Turnstile Siteverify for public signup |
| ICE | `GET /api/v1/node/ice` returns cached short-lived Cloudflare TURN credentials or the public STUN fallback |
| Accounts | SQLite users/passkeys/ceremonies/sessions; passkey setup, login, register, step-up; fresh-passkey account deletion with `Deleted account` attribution; durable workspace enrollment tokens; API signing keys with scopes/expiry; `/api/v1/me`; authenticated `/api/v1/events` and device reads |
| Remote MCP | OAuth metadata, dynamic client registration, authorization code + S256 PKCE with explicit consent, rotating refresh tokens, resource-bound Bearer tokens, scoped standard JSON-RPC `/mcp` tools |
| Authority | Passkey-authorized control clients, Owner-signed MCP grants, API signing keys, schedule permits, canonical workspace snapshots, signed transitions, offline delivery, Node lock bootstrap |
| Node MCP channel | Authenticated Node WebSocket, RC Lock verification, Node-owned process journals, remote image transfer |
| Control data plane | Authenticated challenge/open/WebRTC signaling, Node and CLI/browser encrypted DataChannel sessions, current RC Lock invalidation, API-key execution authority |
| Workspace authority | Owner role management with fresh passkey step-up, invites and join links, access/activity pages, workspace lifecycle, and durable bounded audit events |
| Upgrade | Release API, digest pinning, size bounds, semver/downgrade rules, `minimumUpgrader`, core-profile validation, atomic replace, journal recovery, Windows side-by-side activation |
| Diagnostics | Bounded local journal, service log tail/follow, process-tree inspection, `doctor` health checks |
| Service | Per-user systemd, launchd, and Scheduled Task install/start/stop/uninstall |
| Release bundles | `rc-<os>-<arch>.tar.gz`, `rc-core-profile.tar.gz`, `rc-core-components.tar.gz`, `rc-upgrade.json`, `install.sh`, `install.ps1` |
| Installers | Single-`rc` archive, profile and component digest validation, local override preservation, rollback, legacy bundle fallback; POSIX smoke and release install smoke on Linux/macOS/Windows |

## Deprecated and operational notes

- **SSH gateway**: deprecated and removed. All remote terminal and execution
  workflows use direct E2EE WebRTC channels.
- **Execution history**: deprecated and removed. Process state is strictly
  ephemeral in memory for live leases and reconnection reconciliation.
- **E2E coverage**: the browser product E2E passes on Linux CI and Windows CI,
  and locally on macOS. It now also completes a real Remote MCP connection:
  dynamic client registration, PKCE authorization, passkey consent, RC Lock
  sync, `tools/list`, Node `process_run`, and grant revocation. The Windows
  suite runs encrypted browser control and process exit on a `windows-latest`
  runner. `RC_BROWSER_E2E_ENABLED` and `RC_WINDOWS_RUNTIME_ENABLED` are enabled
  in the repository.
- **Package management scope**: the component host implements
  `component-store.installed` so `rc list` reports managed components.
  `rc add`, `rc install`, package-source resolution, artifact caching, and
  managed state are outside the control-authority scope and fail with an
  explicit message. `rc update` remains the native component-update command.
- **Native host platform scope**: the native component host is supported on
  Linux and macOS. Windows runs the control-plane and browser E2E; porting the
  native component host to Windows is out of scope for the control-authority
  model.

## Removed Rust harnesses

Legacy account-store migration carries MCP client registrations and Owner-signed
grants, and an idempotent backfill repairs stores that were migrated before
that coverage existed. Legacy OAuth tokens and codes are intentionally not
imported because their hash encoding differs; MCP clients receive current tokens
after re-authorizing.

The 22 cargo smoke scripts were deleted with the Rust crates. `tests/integration/`
now covers server lifecycle, enrollment, signed presence, CLI behavior, local
shell, logs, doctor, component build/load/reload, API-key authentication,
workspace authority and audit events, sessions and passkeys, key agreement and
mesh envelopes, package-manager discovery, storage, release upgrade (happy path,
no-op, downgrade, minimum upgrader, digest mismatch, local override), and
fail-closed routes. Kernel host imports for mesh policy, key custody, identity,
sessions, API credentials, events, and `component-store.installed` are
implemented; the integration suite invokes each control-authority fixture
command through the real component host.

## Enabling gated jobs

- `RC_BROWSER_E2E_ENABLED=true` enables `bun run smoke:browser-e2e`.
- `RC_WINDOWS_RUNTIME_ENABLED=true` enables the Windows component/CLI harness
  and `scripts/windows-browser-e2e.ps1`.

Do not set them to make a run green; they assert behavior the current runtime
does not provide.
