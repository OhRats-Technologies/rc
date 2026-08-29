# RC component migration roadmap

This file is the implementation checklist for turning RC into a small native
kernel plus independently built WebAssembly components. It is updated in the
same commits that complete its tasks.

## End state

The native `rc` kernel owns only:

- Wasmtime and the WIT linker;
- component discovery, lifecycle, dependency reconciliation, generations, and
  resource limits;
- capability-scoped adapters to operating-system resources;
- top-level CLI dispatch and a recovery surface;
- atomic replacement and re-exec of the native kernel.

RC product behavior lives in Wasm components. The official components remain
in this monorepo but have independent manifests, lockfiles, builds, versions,
and release artifacts. A component implementation change builds only that
component. A kernel implementation change cross-builds only the kernel. A WIT
change rebuilds the packages that import it. A profile change assembles a new
component set without compiling code.

Installed local components are trusted by placement in the RC component
directory. Wasm imports still enforce least privilege between trusted
components. Remote artifacts are resolved before placement, pinned by digest,
and may come from a package catalog, OCI registry, GitHub repository/subpath,
URL, archive, or local path.

The user-facing package commands are:

```text
rc add <spec>
rc remove <name>
rc install
rc outdated [name...]
rc update [name...] [--latest]
rc list
rc upgrade
```

`update` changes components. `upgrade` changes the native kernel.

## Architectural decisions

- [x] Keep kernel, WIT, build tooling, profiles, and official components in the
  main RC monorepo.
- [x] Treat repositories as package containers, not package identities.
- [x] Use WIT package identities for runtime interfaces and component
  capabilities.
- [x] Target stable WASIp2 first; expose RC-owned WIT interfaces so WASI can be
  changed without changing every component contract.
- [x] Use declarative catalogs for friendly names and OCI for immutable remote
  artifacts. Local paths and direct GitHub/URL/OCI specs remain supported.
- [x] Keep Bun only inside a component that needs it. Do not make Bun part of
  kernel or unrelated component builds.
- [x] Preserve the current public product behavior while each domain moves,
  then delete the replaced implementation rather than carrying two systems.
- [x] Keep the Coolify `/data` volume and Cloudflare ingress model. Production
  remains one application until the componentized runtime is proven.

## 0. Baseline and controls

- [x] Inventory the current Rust workspace, browser build, release workflows,
  production Dockerfile, and infrastructure handbook.
- [x] Replace repository rules that mandate statically linked product logic
  with kernel/component invariants.
- [x] Add a component/WIT source-size and dependency-boundary check.
- [x] Record the current production smoke test as a migration acceptance test.

Acceptance: contributors can tell which directory owns a change, and CI can
reject accidental coupling between the native kernel and component source.

## 1. Native kernel and component lifecycle

- [x] Create `kernel/` as an independent Cargo workspace and lockfile.
- [x] Define the first lifecycle/CLI WIT world under `wit/`.
- [x] Load `.wasm` components from a configurable trusted directory.
- [x] Cache Wasmtime compilation persistently in a private, bounded kernel cache.
- [x] Validate component IDs, semantic versions, provided services,
  requirements, and command descriptors.
- [x] Reconcile components through loaded, waiting, active, and failed states.
  Generation draining for long-lived resources is tracked with update delivery.
- [x] Keep the last healthy generation active when replacement activation
  fails.
- [x] Deactivate dependents when requirements disappear and reactivate them
  when providers return.
- [x] Discover top-level CLI commands from active components and reject command
  collisions deterministically.
- [x] Add bounded memory, fuel interruption, and no ambient filesystem or
  network access by default.
- [x] Add a debounced directory watcher that rescans desired state instead of
  executing raw filesystem events.
- [x] Add kernel-owned `--help`, `--version`, component inspection, and repair
  commands.
- [x] Build provider/consumer fixtures and test add, remove, replace, rollback,
  dependency loss, traps, and resource exhaustion.

Acceptance: copying, replacing, or deleting a fixture component changes the
live graph without restarting the kernel, and a broken replacement leaves the
previous generation usable.

## 2. Independent build graph

- [x] Give each Rust component its own Cargo workspace and lockfile.
- [x] Add one command that builds a named component to a deterministic output
  path.
- [x] Add a component manifest schema and validator.
- [x] Add a changed-unit resolver for kernel, WIT packages, SDKs, components,
  profiles, and documentation.
- [x] Split CI into kernel, dynamically generated component, profile assembly,
  documentation, and scheduled full-build jobs.
- [x] Cross-build native release artifacts only when kernel inputs change.
- [x] Build each affected Wasm component once, without an OS/architecture
  matrix.
- [x] Cache kernel and each component independently.

Acceptance: a component-only commit does not compile or cross-build the kernel,
and a kernel build unit does not compile product components. The separate
component-runtime job may consume already-built fixtures or build them as
kernel integration-test inputs.

## 3. Package state and sources

- [x] Define versioned `rc.toml` desired-state and `rc.lock` serialization.
- [x] Make `install` replay the exact locked digests from a content-addressed
  cache rather than re-resolving mutable sources.
- [x] Implement `add`, `remove`, `install`, `list`, `outdated`, and `update` as
  component-provided commands.
- [x] Define the typed package-source WIT interface and keyed provider registry.
- [x] Implement a local-path source provider as an independently built
  component.
- [x] Implement HTTP(S), GitHub repository/subpath, and OCI source
  providers as components.
- [x] Support source forms such as:

  ```text
  ohrats:webui@^1
  github:OhRats-Technologies/rc//components/webui#<revision>
  oci:ghcr.io/ohrats-technologies/rc/webui@sha256:<digest>
  https://example.invalid/webui.wasm
  ./webui.wasm
  ```

- [x] Inspect candidates before placement and use atomic same-filesystem
  placement/removal.
- [x] Cache artifacts by digest outside the trusted component directory.
- [x] Keep manually copied components visible as unmanaged and never replace
  them during managed updates.
- [x] Publish independently versioned components to an OCI-compatible registry
  and attach build provenance/SBOM metadata when available.
- [x] Add declarative catalogs for friendly package names; catalog files never
  execute code.

Acceptance: a clean machine can reproduce an exact component set from
`rc.toml` and `rc.lock`, while local component development remains a simple
copy or local-path install.

## 4. Diagnostics vertical slice

- [x] Define typed diagnostics event, query, reporting, WebUI-slot, and
  authorization-gated mesh WIT packages.
- [x] Move structured logging/error collection into an official diagnostics
  component.
- [x] Add a diagnostics CLI component (`rc doctor`, local logs/status).
- [x] Prove capability isolation: diagnostics receives metadata but no command
  plaintext, control keys, unrestricted files, or raw sockets.
- [x] Add a diagnostics UI component that activates only when both diagnostics
  query and WebUI slot services exist.
- [x] Add an optional mesh diagnostics provider for explicitly authorized
  cross-node reports.

Acceptance: [x] diagnostics can be installed, removed, replaced, and extended
without a kernel build or restart; capability checks keep its WIT imports
metadata-only.

## 4.5. RC 0.18 dogfood release

- [x] Ship the existing native Node/controller path together with the new
  Wasmtime kernel rather than replacing working remote control prematurely.
- [x] Install package-manager, local/HTTP/GitHub/OCI source providers, and
  local diagnostics as a portable core component bundle.
- [x] Dispatch component-provided top-level commands through `rc` while
  retaining native Node/controller commands until their WIT ports are ready.
- [x] Make `rc update` update managed components and `rc upgrade` refresh the
  native platform/core bundle.
- [x] Publish four native `rc` assets, four native kernel assets, and one
  portable core-component bundle without rebuilding Wasm per platform.
- [x] Install 0.18 on a real enrolled Mac, verify the component graph and
  package commands, then execute a GUI process through production RC.

Acceptance: 0.18 can be dogfooded on an enrolled machine without losing the
existing browser/CLI WebRTC control path, while component lifecycle and package
commands run through the installed Wasm kernel.

## 5. Componentized web runtime and `rc.ohrats.party`

- [x] Define typed HTTP gateway, session, WebAuthn, UI slot, static
  asset, and event-stream WIT packages.
- [x] Implement the native HTTP listener as a capability-scoped kernel adapter
  and route requests to active components.
- [x] Move the exact canonical landing page and complete public documentation
  catalog into the `ohrats:webui-shell` component without reconstructed copy or
  layout.
- [x] Move sign-in/setup surfaces into identity-backed component routes.
- [ ] Move passkey ceremony handling, authenticated shell, sidebar, account,
  workspace, device, API, MCP, CLI authorization, and error views into
  componentized routes as their domain services become available.
- [x] Keep server-rendered HTML for the public shell and registered page
  contributions.
- [ ] Move progressive browser enhancements into the WebUI component. Bun may
  remain its private browser-asset tool until replacing it has a measured
  benefit.
- [x] Embed/fingerprint WebUI-owned assets in its component artifact while
  continuing to consume company-wide immutable assets from
  `assets.ohrats.party`.
- [x] Define and exercise WebUI page slots with the diagnostics UI component.
- [x] Add sidebar, device-panel, and settings-panel contribution interfaces.
- [ ] Delete the replaced native page renderer/routes and the global browser
  build lane.

Acceptance: [ ] passkey login/setup and authenticated browser flows run through
components in production. The public landing/docs runtime and live route
contribution smoke tests already pass through the native kernel listener.

## 6. Canonical server domain components

- [x] Move durable storage behind a typed storage interface. Prove SQLite
  locking, transactions, backup, and crash recovery under the chosen boundary;
  use a narrow native storage adapter if direct WASI SQLite is unreliable.
- [ ] Port identity/passkeys/sessions and API/CLI credentials.
  - [x] Add a keyed, stateless ES256 WebAuthn verifier component with typed
    credential state and deterministic registration/authentication fixtures.
  - [x] Add component-owned durable users, browser sessions, and single-use
    ceremony state; prove restart recovery, expiration, revocation, provider
    withdrawal, and that raw bearer tokens never enter durable storage.
  - [x] Add typed component-owned API/CLI credential state with Ed25519
    proof-of-possession, canonical request binding, bounded durable nonce replay
    protection, scopes, expiration/revocation, and passkey-stepped CLI approval.
  - [x] Require identity-issued, one-use, operation-bound passkey authorization
    tokens for component API/CLI credential creation, approval, and revocation.
  - [ ] Route production API/CLI credential administration through those
    services, then remove the remaining native SQLite paths.
  - [x] Route setup/login WebAuthn registration and authentication ceremonies,
    browser sessions, logout, and restart persistence through identity components.
- [ ] Port workspaces, membership, invitations, and authority snapshots.
  - [x] Add the typed workspace/membership/invitation package and durable
    workspace-store component, including atomic Personal workspace creation,
    Owner and membership invariants, non-Owner expiring/single-use hashed
    invitations, deletion cleanup, and restart smoke coverage.
  - [x] Add typed key/crypto/authority foundations plus a durable authority-lock
    component with exact canonical snapshots, one-time TOFU initialization,
    exact-parent generations, Owner control-key verification, trusted Node keys,
    active MCP grant hashes, and live-session invalidation signals.
  - [ ] Port and locally enforce signed authority snapshots.
- [ ] Port device enrollment, revocation, presence, and Node rendezvous.
  - [x] Define and exercise component-owned durable device registry, hashed
    one-time enrollment, immutable Node keys, tombstone revocation, presence
    leases, and transport-neutral rendezvous metadata services.
- [ ] Port control authorization, TURN provider, signaling, and events.
  - [x] Add a typed lifecycle-event component with durable monotonic cursors,
    authorization-scoped filters, idempotent appends, bounded retention, and
    restart/gap recovery fixtures. Live HTTP event streaming remains pending.
- [ ] Port MCP OAuth and the five-tool process harness.
- [ ] Port the OpenSSH gateway integration.
  - [x] Add component-owned durable SSH public-key credentials and typed policy
    decisions for immutable device routing, workspace roles, forced commands,
    forwarding prohibitions, and bounded sessions. Native sshd/bridge adapters
    remain to be reduced to consumers of these contracts.
- [ ] Define the canonical server profile and exact lockfile.
- [ ] Replace the giant native `AppState` with component services and host
  resources.
- [ ] Delete `rc-server` after its final domain has moved.

Acceptance: `rc.ohrats.party` is the native kernel plus the canonical locked
component graph, using the existing Coolify volume, ingress, and public routes.

## 7. Node, controller, and transport components

- [x] Define typed process and transport WIT for process approval, access,
  terminal sizing, signals, authorization/termination timing, buffer/input
  limits, ICE attempts, answer planning, timeouts, and route classification.
- [x] Require component process/transport services for the Node runtime with no
  native product-policy fallback.
- [x] Carry the host-only, STUN-direct, then TURN-relay plan in the Node-signed
  control handshake for browser and CLI adapters.
- [x] Define the remaining typed key, crypto, authority, and mesh WIT
  packages using resource handles for secrets, processes, streams, and
  connections.
- [ ] Port crypto suites and key-custody adapters into independently selectable
  components.
- [ ] Port RC Lock and execution authorization.
- [x] Move process authorization expiry and TERM-to-KILL lifecycle policy out
  of the bounded OS process adapter.
- [x] Port WebRTC attempt and answer policy as the first transport provider.
- [x] Add a second test transport to prove provider selection and replacement
  before implementing QUIC.
- [ ] Port mesh routing, capability advertisement, and artifact cache services.
- [ ] Port account/controller and Node service commands into CLI components.
- [ ] Define Node and controller profiles.
- [ ] Delete `rc-node`, `rc-cli`, `rc-crypto`, `rc-mesh`, `rc-context`, and
  `rc-protocol` once their final behavior lives behind WIT components.

Acceptance: kernel builds are platform-specific; all product transports,
crypto, authority, process policy, Node behavior, and controller commands are
replaceable components.

## 8. Updating and distribution

- [x] Implement component update policy as a package-manager/updater component.
- [x] Resolve/download/compile candidates before changing the active graph.
- [x] Route new calls/sessions to a healthy new generation while old sessions
  drain on the previous generation.
- [x] Define explicit state snapshot/restore only for components that need it;
  prefer durable external state and generation draining.
- [x] Implement `rc upgrade` through an updater component plus one kernel-owned
  atomic replace/re-exec primitive.
- [x] Install the kernel and a recoverable core component bundle from
  `install.sh`.
- [x] Allow local and mesh caches to satisfy digest-addressed artifact fetches
  before an OCI registry is contacted.

Acceptance: changing one component updates only that component with no service
restart when its state model permits; changing the kernel leaves compatible
components installed and reuses them after re-exec.

## 9. Production cutover and cleanup

- [ ] Build a production image containing the kernel, canonical profile lock,
  and its exact official component artifacts.
- [ ] Preserve `/data`, non-root execution, current Cloudflare/Coolify ingress,
  SSH loopback behavior, and health endpoints.
- [ ] Extend container and browser smoke tests to verify the active component
  graph and exact artifact digests.
- [ ] Deploy to the existing Coolify RC application and verify the active Git
  SHA, component lock digest, public pages, passkeys, Node presence, direct
  control, MCP, SSH, and installer.
- [ ] Update the infrastructure handbook to describe kernel/component releases,
  profile locks, OCI artifacts, and rollback by exact graph digest.
- [ ] Remove obsolete global versions, release jobs, Docker stages, Bun tasks,
  source trees, documents, and compatibility scaffolding.
- [ ] Run an architecture audit ensuring the native kernel contains no RC
  product-domain types or policy.

Acceptance: production runs the component architecture exclusively, the old
monolith is deleted, and build/deploy/update behavior matches the end-state
invariants at the top of this document.

## Iteration log

- 2026-08-29: replaced the authority fixture's generic signature verifier with
  a production `crypto-ed25519` component that owns Ed25519 verification and
  SHA-256. Authority runtime smoke now proves verifier dependency activation
  and provider withdrawal. Protected-key custody and control-session crypto
  remain the completion gate for the broader crypto migration item.
- 2026-08-29: completed the remaining typed runtime contracts needed by the
  Node/transport migration. Process WIT now defines host-owned opaque process
  and stream resources alongside the existing secret-key and connection
  handles. A general mesh package owns bounded capability negotiation,
  trusted-topology route planning, and opaque-envelope forwarding policy;
  deterministic component smoke coverage proves provider withdrawal and
  rejects noncanonical capability input, expired envelopes, and relay loops.
- 2026-08-29: audited component state migration and found no component that
  requires an in-memory snapshot ABI. Durable domain stores already externalize
  restart state, host-owned bounded config/state adapters cover bootstrap data,
  and pinned runtime generations drain in-flight calls before deactivation.
  Snapshot/restore remains intentionally absent until a component demonstrates
  state that cannot fit either durable external storage or generation draining.
- 2026-08-29: replaced caller-constructed API/CLI administrator records with
  identity-issued random human-authorization tokens whose durable records are
  digest-keyed. Identity binds each one-use claim to the browser session, client,
  operation, fresh passkey assertion, and expiry; runtime smoke covers forged and
  operation-mismatched proofs, one-use authorization replay, signed-request
  replay races, credential expiry, restart persistence, and provider withdrawal.
  Production HTTP routing and native SQLite removal remain the cutover gate.
- 2026-08-29: completed typed cache routing for package installation. The
  package manager walks priority-ordered local/mesh cache providers by digest
  before resolving the locked source, verifies and persists cache hits through
  the kernel content-addressed store, and only reaches OCI after cache misses.
  The production local provider now uses the narrow kernel storage adapter;
  runtime fixtures retain RC-Lock-authorized mesh denial/fallback coverage.
- 2026-08-29: moved `rc upgrade` policy into the updater component and limited
  the kernel to a digest-pinned, bounded-health, journaled atomic replacement
  primitive. Interrupted activation rolls back on restart, successful restart
  acknowledges and removes the backup, same-digest updates are no-ops, and the
  exact updater artifact now ships in the recoverable core profile.
- 2026-08-29: replaced the installer’s second native-upgrade step with one
  digest-locked bootstrap of `rc`, the kernel, and an exact core profile. It
  validates archive shape and every component digest before activation,
  preserves local overrides, and retains the previous native/core generation
  for rollback with installer-specific ownership markers.
- 2026-08-29: reconciled the WebUI slot checklist with the implemented typed
  shell contract. Caller-owned sidebar, device-panel, and settings-panel
  contributions are bounded, deterministically ordered, withdrawn on provider
  removal, and rendered only in their selected authenticated-page context.
- 2026-08-29: strengthened the authority-lock fixture with an actual signed
  generation transition: a deterministic control key signs the exact canonical
  parent/next payload, replay is rejected, restart preserves generation one,
  and the live-session invalidation epoch is consumed exactly once.
- 2026-08-29: added typed opaque key handles, crypto verification, and canonical
  workspace authority contracts. The authority-lock component durably preserves
  TOFU state, validates exact-parent Owner transitions through a typed Ed25519
  verifier, includes Node identity/transport keys, and emits invalidation epochs.
- 2026-08-29: added the typed API/CLI credential component foundation. It stores
  public keys and policy metadata only, verifies method/path/query/timestamp/
  nonce/body-bound Ed25519 requests, commits nonce use with optimistic CAS, and
  proves restart, tamper, replay-race, expiry, revocation, and one-use CLI flow.
- 2026-08-29: made package-manager updates graph-transactional. Every selected
  artifact is resolved, digest-bound, cached, inspected, and prepared before a
  fenced publication; host-side journals recover or roll back interrupted
  commits, preserve unmanaged files, and keep prepared transactions isolated.
- 2026-08-29: added generation leases to the kernel service registry. Healthy
  replacements receive new calls immediately, withdrawn generations reject new
  pins, and existing streamed HTTP sessions retain the old generation until
  their final chunk or disconnect before dependent-first deactivation.
- 2026-08-29: aligned the linked architecture, capability, context, and mesh
  specifications with the WIT/Wasmtime end state. Transitional `rc-context`,
  `rc-mesh`, native product crates, and the global browser tree are now
  explicitly documented as deletion queues rather than the target plugin ABI.

- 2026-08-28: inventoried the repository and handbook; selected a monorepo,
  WASIp2, WIT-first component ABI, OCI artifact plumbing, declarative catalogs,
  Bun-like package commands, and Coolify-preserving migration.
- 2026-08-28: implemented the independent Wasmtime kernel, trusted-directory
  reconciliation, semver-gated services, component-provided CLI commands,
  rollback-safe replacement, capability limits, and adversarial lifecycle
  fixtures. The existing container/browser/API smoke matrix remains the
  production acceptance baseline until each domain moves.
- 2026-08-28: added independently locked component manifests, deterministic
  build and profile plans, an affected-unit resolver with tests, per-unit CI
  caches/matrices, a weekly full integrity build, and kernel-only four-platform
  release assets. Local cross-builds produced valid Intel macOS and Linux
  kernels.
- 2026-08-28: added dynamic typed WIT service forwarding, keyed provider
  selection, component-private state, an atomic managed-component store, a
  local-file source component, and component-provided package commands. The
  package smoke test proves managed updates while preserving manually copied
  components.
- 2026-08-28: completed the package layer with exact lockfile replay from a
  SHA-256 cache, semver catalogs, bounded HTTP, GitHub monorepo/release, and OCI
  source components, authenticated OCI fixture coverage, and an independent
  GHCR component release workflow with build provenance.
- 2026-08-28: completed the diagnostics vertical slice with a bounded metadata
  store, reporter, `doctor`/`logs` CLI, reactive WebUI page contribution, and a
  mesh report component that cannot activate without an authorization service.
  A live watcher test proves dependency-driven activation and withdrawal.
- 2026-08-28: added the native component HTTP adapter and moved the public RC
  landing, sign-in/setup surfaces, docs shell, fingerprinted CSS, and
  diagnostics page route into the WebUI component. Live HTTP tests prove route
  providers can appear and disappear without restarting the listener.
- 2026-08-28: added a component-private typed durable store backed by the
  kernel's narrow SQLite adapter. Unit and runtime tests cover namespaces,
  optimistic transactions, writer locking, abrupt rollback, persistence, and
  consistent online backup. The OpenSSL-backed `webauthn-rs` implementation was
  rejected for WASIp2.

- 2026-08-28: added an identity HTTP component with exact setup/login views,
  ES256 registration and authentication through the keyed verifier, atomic
  user/passkey storage, HttpOnly browser sessions, setup-token configuration,
  reliable logout, and persistent-Chrome setup/restart/login coverage.
- 2026-08-28: added a keyed ES256 WebAuthn verifier component using a pinned
  pure-Rust relying-party core. Typed WIT carries RP policy and portable COSE
  credential state; unit and runtime fixtures prove registration,
  authentication, user-handle binding, counter advancement, tamper rejection,
  algorithm routing, and dependency withdrawal. Ceremony/session policy remains
  in the identity migration.
- 2026-08-28: moved Node process authorization/validation and WebRTC ICE
  planning into independently built typed components. Browser and CLI control
  use the Node-signed host/STUN/TURN plan; the native host fails closed when a
  required policy service is absent.
- 2026-08-28: added a generic typed streamed HTTP provider contract and kernel
  adapter with pinned provider sessions, bounded incremental chunks, delayed
  polling, disconnect close notification, finite-route fallthrough, and SSE
  lifecycle/concurrency fixtures. WebSocket transport remains separate work.
- 2026-08-28: audited the Node policy migration from the public release through
  the production control plane and enrolled Mac. Typed component calls,
  independent hot replacement, direct/STUN/TURN coverage, process and MCP
  harnesses, launchd reconnect, public installation, and browser direct WebRTC
  passed; RC 0.19.2 pins production to the audited release commit.
