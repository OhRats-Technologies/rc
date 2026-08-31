# RC Runtime Migration

Items are checked only after acceptance evidence passes. docs/RUNTIME.md is normative.

## Specification
- [x] Preserve and inventory pre-existing user changes.
- [x] Map runtime, client, MCP, platform, service, installer, and release assumptions.
- [x] Pin Bun Shell upstream and define extraction provenance.
- [x] Specify execution modes, lifetimes, privacy, Windows, and scheduler authority.
- [x] Add executable native-host, component-runtime, shell, MCP, browser-control, scheduler, and release conformance fixtures.

## Contracts and runtime
- [x] Add process WIT v0.3 exact argv, typed modes/signals, environments, lifetimes, groups, and byte streams.
- [x] Link narrow process, filesystem, environment, and clock host capabilities in the kernel world.
- [x] Publish separately linkable shell and scheduler contracts.
- [x] Publish and build a resource-backed execution-runtime service over policy and process-host.
- [x] Emit bounded diagnostics for runtime platform/process/terminal backends, lifetime-specific active counts, and scheduler timezone/overlap policy without execution plaintext.
- [x] Probe exact argv end-to-end through execution-runtime, process-host, and the native OS backend.
- [x] Probe a typed native login-shell PTY in `rc doctor`, grant its narrow environment capability, and surface bounded startup failures instead of an unexplained exit 127.
- [x] Route portable RC Shell jobs through the production execution manager and shell component service.
- [x] Inject the component-backed execution manager into the production kernel Node path.
- [x] Delete the production and test-only `rc-process-runner` handoff and transitional native `ProcessManager` after process-host parity passed.
- [x] Add `execution-v2` negotiation and reject unsupported execution safely.
- [x] Unify browser reattach and MCP cursor reads on the Node output journal.
- [x] Revalidate MCP grant, device, process ownership, and action policy at the Node.
- [x] Remove hosted MCP process routing state; route every operation with device ID and bind Node executions to the originating grant ID across hosted restarts.
- [x] Prove a fresh hosted MCP correlation hub can query an existing Node-owned execution after the previous hub is dropped.
- [x] Terminate active MCP/scheduled executions when durable RC Lock authority is revoked and bound MCP runtime to grant expiry.
- [x] Delete the hosted MCP process-output buffer.
- [x] Bound hosted MCP status/operation and image correlation maps without evicting active requests.
- [x] Add separately linkable filesystem, environment, clock, shell, runtime, and scheduler capability contracts.
- [x] Keep portable environment policy case-sensitive and enforce `PATH`/`Path` alias conflicts only at the Windows host boundary.
- [x] Implement and link Unix process-host with exact argv, PTY, streams, signals, and whole-tree parity.
- [x] Implement component-owned execution-ID claims, unified binary journal, lease state, and at-most-once behavior; retain only bounded native resource routing.
- [x] Persist a bounded component-owned replay filter before spawn so ambiguous execution IDs remain non-replayable across Node restarts and component generations without storing execution content.
- [x] Prove the replay filter rejects the same execution ID after a fresh kernel process opens the same Node-local durable store.
- [x] Enforce process-policy access, resize normalization, and semantic signal authorization inside execution-runtime after start.
- [x] Reject channel/lifetime confusion and PTY requests on MCP or scheduled execution at the process-policy boundary.
- [x] Enforce a 256 MiB Node-wide journal budget by rejecting new executions rather than evicting live execution state.
- [x] Test component-owned attached/managed/scheduled lease deadlines, writer supersession, max runtime, and TERM escalation with a fake clock.
- [x] Migrate typed encrypted browser/CLI start modes and Node capability negotiation.
- [x] Migrate MCP status/input/signal correlation to Node-owned journals and delete hosted process buffering.
- [x] Preserve arbitrary MCP process bytes with explicit text/base64 input and output encodings.
- [x] Reject malformed Node output encodings and unknown stream labels at the MCP boundary instead of silently dropping or relabeling chunks.
- [x] Preserve each Node journal chunk's absolute cursor in MCP structured output and its exact schema.

## Portable shell
- [x] Audit and pin Bun Shell commit `ed950b88ab2ec6b58bccdfe7d310731b8ca13c4d` with MIT provenance.
- [x] Add a stable-Rust Bun-derived parser/AST/expansion core and conformance corpus.
- [x] Publish and build the first-party `ohrats:rc-shell/compiler@0.1.0` WASIp2 component.
- [x] Implement the first-party RC executor over explicit process/filesystem/environment capabilities.
- [x] Implement the executor's exact external-command and portable builtin path with binary stream forwarding.
- [x] Run external pipeline stages in one execution group with bounded binary-safe pumping and drain-before-exit behavior.
- [x] Implement binary-safe stdin/stdout/stderr file redirects and append behavior for single shell commands.
- [x] Apply stdout/stderr redirects at every pipeline stage without leaking redirected bytes into the next stage.
- [x] Implement asynchronous `&&`, `||`, and `;` chains within one top-level execution group.
- [x] Persist assignment/export/unset/cd state across shell chains and add portable filesystem builtins.
- [x] Implement `yes` as a bounded streaming portable builtin and prove it remains cancellable inside a pipeline.
- [x] Support portable builtin producers/consumers inside binary-safe pipelines.
- [x] Run bounded asynchronous builtin/external/pipeline/nested command substitution in the top-level execution group.
- [x] Prove KILL cancellation of an already-running portable-shell pipeline through execution-runtime and the shared native execution group.
- [x] Run portable builtin pipeline cancellation plus redirect/append/intermediate-stage probes in the common native policy check on every OS instead of gating them to Unix.
- [x] Add a binary external-process-to-portable-builtin pipeline probe to the common native policy check on every OS.
- [x] Add an external-process command-substitution probe to the common native policy check on every OS.
- [ ] Pass the component-backed expansion, pipeline, redirect, glob, builtin, and cancellation corpus on Linux, macOS, and Windows native runners.

## Windows
- [x] Add centralized Windows/Unix state, data, component, cache, binary, home, and executable-name resolution.
- [x] Add a Windows `process-host` backend with Job Object ownership, piped streams, and ConPTY resize/merged output.
- [x] Kill and reap a newly spawned Windows child if Job Object assignment fails, with a native leak regression test.
- [x] Run Windows process-host tests through a real guard fixture instead of accidentally spawning the Rust test harness as the guarded process.
- [x] Gate ConPTY target creation behind successful Job assignment so terminal descendants cannot escape during the post-spawn ownership window.
- [x] Add native Windows TERM process-group and ConPTY Ctrl-C signal regressions.
- [x] Add native Windows binary stdout/stderr and exact stdin/EOF regressions.
- [x] Add native Windows Unicode cwd plus clean/set/unset environment regression coverage.
- [x] Add cross-platform run-lock semantics using exclusive Windows file sharing.
- [x] Replace sensitive-state permission no-ops with protected and validated Windows DACLs.
- [x] Route hosted-server private data-directory protection through shared Windows DACL/Unix permission enforcement instead of a non-Unix no-op.
- [x] Reject null and empty Windows DACLs without dereferencing an empty ACL entry array.
- [x] Add native `windows-latest` workspace/kernel CI gates and Windows-only runtime/ACL tests.
- [x] Cross-check Windows kernel, CLI, Node, and platform targets with an MSVC SDK and fail on Windows-only warnings before native CI.
- [x] Make Win32 execution-group ownership safely transferable into Wasmtime's `Send` host state and expose the execution guard through a narrow kernel entry point.
- [x] Add native macOS workspace/kernel gates and execute the same component-backed runtime/shell policy corpus used on Linux and Windows.
- [x] Pass the workspace, kernel, and complete component-backed runtime/shell policy corpus on a native macOS 15 runner with Rust 1.98.0.
- [x] Monitor native Windows console dimensions and send typed terminal resize messages only when the window size changes.
- [x] Add Windows-native platform-root fixtures for `%LOCALAPPDATA%\\OhRats\\RC`, executable suffixes, missing-directory failure, and Unicode browser-target encoding.
- [x] Reject interior NULs before passing browser authorization targets to `ShellExecuteW`.
- [x] Make workspace and kernel all-target Clippy gates pass with warnings denied.
- [x] Implement per-user Windows Task Scheduler install/start/stop/status/uninstall for logged-on users.
- [x] Document that the Windows per-user task requires an interactive logged-on session and is not an unattended LocalSystem service.
- [x] Produce and release-gate native Windows AMD64 CLI/kernel archives with canonical `.exe` members.
- [x] Make Windows release target verification shell-independent and trigger kernel artifacts when shared platform code changes.
- [ ] Pass native Windows Job Object, pipes, ConPTY, Unicode spawn/environment, resize, signal, and whole-tree termination tests.
- [ ] Pass native Windows platform-directory, executable-lookup, run-lock, console, and browser-opening tests.
- [x] Implement and validate restrictive ACLs for sensitive state.
- [x] Protect trusted component placement plus kernel state/cache/catalog roots with private Unix permissions or Windows DACLs before reconciliation.
- [x] Classify Windows reparse points as link-like and prove recursive filesystem operations do not traverse directory links.
- [x] Implement per-user Task Scheduler service management.
- [ ] Pass native Windows PowerShell install, side-by-side activation/rollback, release-asset, and CI acceptance gates.
- [x] Add digest-pinned PowerShell install with bounded archive validation, profile verification, no-downgrade, rollback, and release publication.
- [x] Bind every PowerShell installer asset URL to the selected immutable release tag/name and reject HTTPS-to-HTTP redirects.
- [x] Replace existing Windows activation pointers with `File.Replace` and native-CI-check replacement and temporary-file cleanup.
- [x] Apply and native-CI-check protected installer DACLs before writing Windows runtime, component, state, rollback, binary, or staging artifacts.
- [x] Make PowerShell-installer changes trigger native Windows CI and exact-SHA release prebuilds.
- [x] Add Windows side-by-side kernel versions with an atomic active-runtime pointer and restart into the selected generation.
- [x] Fail closed instead of exposing the generic in-place native replacement/re-exec primitive on Windows; direct callers to side-by-side `rc upgrade`.
- [x] Replace in-place running Windows CLI activation with a staged post-exit PowerShell helper.
- [x] Journal Windows installer and `rc upgrade` activation after a complete rollback snapshot and recover interrupted activation before new work.
- [x] Validate independently versioned native kernels semantically and reject kernel downgrades in `rc upgrade` and the PowerShell installer.
- [x] Persist the previous Windows activation and conservatively remove stale semantic-version generations while preserving active, rollback, and unknown directories.

## Scheduler and cutover

- [x] Make reinstall-with-token idempotent: preserve an existing enrollment, leave the token unused, and print actionable guidance.
- [x] Split WebUI enrollment output into full-width new-install and already-installed commands.
- [x] Document one default background enrollment per OS user and explicit state-directory isolation for advanced foreground Nodes.

- [x] Make native upgrade completeness semantic so an equal-version upgrade repairs missing or unloadable required runtime services.
- [x] Validate the complete execution/policy/scheduler/transport service graph before installer or updater activation.
- [x] Make `rc repair` report required-service failures instead of treating parseable component files as sufficient.
- [x] Publish minimum-upgrader metadata for future native core-profile transitions and retain one-time verified-installer migration guidance.
- [x] Treat canonical core-profile members as an atomic native runtime while preserving unrelated third-party components.
- [x] Publish the typed scheduler definition/evaluator WIT contract using normal execution modes and environments.
- [x] Build the first-party scheduler WASIp2 component with five-field cron and bundled IANA timezone rules.
- [x] Define deterministic wall-clock occurrence IDs and test spring-forward skip/fall-back at-most-once behavior.
- [x] Bind schedule permits in RC Lock to schedule, device, Owner, immutable spec hash, runtime, and expiry.
- [x] Persist schedule permit metadata/spec hashes in hosted authority snapshots without persisting schedule execution definitions.
- [x] Implement the clock host and scheduler component with protected Node-local definitions and cursors.
- [x] Persist scheduler cursors before emitting bounded trigger intents and advance misfires without unlimited catch-up.
- [x] Extract the scheduler state machine for native fake-clock tests covering restart idempotence, overlap, disable/expiry, misfires, and backward clock movement.
- [x] Replace fixed schedule polling with component-calculated next wakeups and management-change notifications; retain only bounded active-run authority checks.
- [x] Submit authorized scheduler triggers through execution-runtime and terminate tracked runs after RC Lock revocation.
- [x] Add versioned RC Lock schedule permits and encrypted CLI list/add/remove/enable/disable management.
- [x] Pass fake-clock, DST, overlap, misfire, restart, revocation, and timeout tests.
- [x] Delete the legacy process runner/manager, shell-string direct-argv conversion, hosted MCP process journal, and non-Unix terminal/permission fallbacks.
- [x] Update architecture, API, install, operations, release, and public quickstart documentation for the portable runtime, Windows, and scheduler.
- [ ] Pass the complete Linux, macOS, and Windows native OS matrices.
