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

- [ ] Give each Rust component its own Cargo workspace and lockfile.
- [ ] Add one command that builds a named component to a deterministic output
  path.
- [ ] Add a component manifest schema and validator.
- [ ] Add a changed-unit resolver for kernel, WIT packages, SDKs, components,
  profiles, and documentation.
- [ ] Split CI into kernel, dynamically generated component, profile assembly,
  documentation, and scheduled full-build jobs.
- [ ] Cross-build native release artifacts only when kernel inputs change.
- [ ] Build each affected Wasm component once, without an OS/architecture
  matrix.
- [ ] Cache kernel and each component independently.

Acceptance: a fixture component-only commit does not compile or cross-build the
kernel; a kernel-only commit does not build fixture components.

## 3. Package state and sources

- [ ] Define `rc.toml` desired state and deterministic `rc.lock` resolution.
- [ ] Implement `add`, `remove`, `install`, `list`, `outdated`, and `update` as
  component-provided commands.
- [ ] Define the package-source WIT interface and source-provider registry.
- [ ] Implement local path, HTTP(S), GitHub repository/subpath, and OCI source
  providers as components.
- [ ] Support source forms such as:

  ```text
  ohrats:webui@^1
  github:OhRats-Technologies/rc//components/webui#<revision>
  oci:ghcr.io/ohrats-technologies/rc/webui@sha256:<digest>
  https://example.invalid/webui.wasm
  ./webui.wasm
  ```

- [ ] Cache artifacts by digest, stage outside the trusted directory, and use
  atomic placement/removal.
- [ ] Keep manually copied components visible as unmanaged and never replace
  them during managed updates.
- [ ] Publish official WIT packages/components to an OCI-compatible registry
  and attach build provenance/SBOM metadata when available.
- [ ] Add declarative catalogs for friendly package names; catalog files never
  execute code.

Acceptance: a clean machine can reproduce an exact component set from
`rc.toml` and `rc.lock`, while local component development remains a simple
copy or local-path install.

## 4. Diagnostics vertical slice

- [ ] Define typed diagnostics event, query, and reporting WIT packages.
- [ ] Move structured logging/error collection into an official diagnostics
  component.
- [ ] Add a diagnostics CLI component (`rc doctor`, local logs/status).
- [ ] Prove capability isolation: diagnostics receives metadata but no command
  plaintext, control keys, unrestricted files, or raw sockets.
- [ ] Add a diagnostics UI component that activates only when both diagnostics
  query and WebUI slot services exist.
- [ ] Add an optional mesh diagnostics provider for explicitly authorized
  cross-node reports.

Acceptance: diagnostics can be installed, removed, replaced, and extended
without a kernel build or restart.

## 5. Componentized web runtime and `rc.ohrats.party`

- [ ] Define typed HTTP gateway, route, session, WebAuthn, UI slot, static
  asset, and event-stream WIT packages.
- [ ] Implement the native HTTP listener as a capability-scoped kernel adapter
  and route requests to active components.
- [ ] Move the landing page, login/setup flows, authenticated shell, sidebar,
  account, workspace, device, API, MCP, CLI authorization, and docs views into
  the `ohrats:webui` component.
- [ ] Keep server-rendered HTML and progressive enhancement. Bun may build the
  WebUI component's browser assets until replacing it has a measured benefit.
- [ ] Embed/fingerprint WebUI-owned assets in its component artifact while
  continuing to consume company-wide immutable assets from
  `assets.ohrats.party`.
- [ ] Define and exercise WebUI slots with the diagnostics UI component.
- [ ] Delete the replaced native page renderer/routes and the global browser
  build lane.

Acceptance: the public landing page, passkey login/setup, existing authenticated
pages, and browser smoke tests run through the WebUI component in production.

## 6. Canonical server domain components

- [ ] Move durable storage behind a typed storage interface. Prove SQLite
  locking, transactions, backup, and crash recovery under the chosen boundary;
  use a narrow native storage adapter if direct WASI SQLite is unreliable.
- [ ] Port identity/passkeys/sessions and API/CLI credentials.
- [ ] Port workspaces, membership, invitations, and authority snapshots.
- [ ] Port device enrollment, revocation, presence, and Node rendezvous.
- [ ] Port control authorization, TURN provider, signaling, and events.
- [ ] Port MCP OAuth and the five-tool process harness.
- [ ] Port the OpenSSH gateway integration.
- [ ] Define the canonical server profile and exact lockfile.
- [ ] Replace the giant native `AppState` with component services and host
  resources.
- [ ] Delete `rc-server` after its final domain has moved.

Acceptance: `rc.ohrats.party` is the native kernel plus the canonical locked
component graph, using the existing Coolify volume, ingress, and public routes.

## 7. Node, controller, and transport components

- [ ] Define typed key, crypto, authority, process, transport, and mesh WIT
  packages using resource handles for secrets, processes, streams, and
  connections.
- [ ] Port crypto suites and key-custody adapters into independently selectable
  components.
- [ ] Port RC Lock and execution authorization.
- [ ] Port process lifecycle and PTY/stream handling; the kernel exposes only
  bounded OS process primitives.
- [ ] Port WebRTC as the first transport provider.
- [ ] Add a second test transport to prove provider selection and replacement
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

- [ ] Implement component update policy as a package-manager/updater component.
- [ ] Resolve/download/compile candidates before changing the active graph.
- [ ] Route new calls/sessions to a healthy new generation while old sessions
  drain on the previous generation.
- [ ] Define explicit state snapshot/restore only for components that need it;
  prefer durable external state and generation draining.
- [ ] Implement `rc upgrade` through an updater component plus one kernel-owned
  atomic replace/re-exec primitive.
- [ ] Install the kernel and a recoverable core component bundle from
  `install.sh`.
- [ ] Allow local and mesh caches to satisfy digest-addressed artifact fetches
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

- 2026-08-28: inventoried the repository and handbook; selected a monorepo,
  WASIp2, WIT-first component ABI, OCI artifact plumbing, declarative catalogs,
  Bun-like package commands, and Coolify-preserving migration.
- 2026-08-28: implemented the independent Wasmtime kernel, trusted-directory
  reconciliation, semver-gated services, component-provided CLI commands,
  rollback-safe replacement, capability limits, and adversarial lifecycle
  fixtures. The existing container/browser/API smoke matrix remains the
  production acceptance baseline until each domain moves.
