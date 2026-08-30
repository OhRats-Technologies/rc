# RC Runtime Specification

Status: normative for protocol capability execution-v2.

MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are requirements. WIT owns
cross-component contracts; this document owns product semantics.

## Architecture

    browser / CLI / MCP / scheduler
      -> control adapters
      -> execution-runtime component
      -> policy / RC Shell / native-shell resolver
      -> process-host
      -> Unix process groups or Windows Job Objects

The kernel MUST expose narrow process, filesystem, environment, clock,
platform-directory, service-manager, and activation mechanisms. Authorization,
leases, journals, shell syntax, and scheduling MUST live in components.
Capabilities MUST remain separately linkable.

An Execution is one authorized at-most-once process tree or shell job. An
Attachment controls streams. A Lease defines termination. The user-visible
entity remains Process.

    allocated -> starting -> running -> exited
                     \          \
                      +-----------> lost

Ambiguous starts MUST become lost and MUST NOT replay. A duplicate ID known by
a live Node MUST return existing state or a duplicate error without spawning.
Before native spawn, execution-runtime records the ID in a fixed-size,
component-owned durable replay filter. It survives Node restart and component
replacement, stores no execution content, and may fail closed on a bounded
false positive; it MUST NOT produce a false negative for a recorded ID. On
reconnect process.sync is authoritative and absent live IDs become lost.

## Execution modes

argv(program,args) preserves exact boundaries without shell parsing. rc run
DEVICE -- ... MUST use it. rc-shell(script) uses the portable component and
MUST NOT dispatch to a system shell. system-shell(command) explicitly requests
native command semantics. system-login-shell requests a native interactive
shell and terminal.

Unix resolution uses valid RC_SHELL, then the enrolled user configured shell,
then /bin/sh, with the selected login convention. Windows uses valid RC_SHELL,
then discoverable pwsh.exe, Windows PowerShell, then cmd.exe, in the enrolled
user service environment. On Windows, RC_SHELL MUST name the cmd, pwsh, or
Windows PowerShell family because their command/login flags differ; unknown
shell families fail instead of receiving Unix `-l` or `-lc` arguments.

## Environment, filesystem, and shell

Environment base is inherit or clean; ordinary execution defaults to inherit.
Ordered changes set or unset names. Values MUST NOT be logged, traced,
diagnosed, or hosted-persisted. Browser/CLI values remain DataChannel plaintext;
MCP values MAY transit bounded hosted memory. Windows compares names
case-insensitively and rejects conflicting spellings such as PATH and Path.
Discovery honors PATHEXT and normal Unix PATH.

The process host MAY apply inheritance without exposing values to policy.
RC Shell is explicitly granted environment and broad resource-based filesystem
capabilities as the enrolled OS user. No unrelated component receives them.
Streams and redirects are binary-safe. Unix symlinks and Windows drives, UNC
paths, separators, reparse points, and case behavior MUST be tested. RC Shell
preserves UNC and drive-rooted paths, resolves root-relative Windows paths
against the shell cwd drive, and distinguishes `C:relative` from
`C:\\absolute`.

RC Shell targets Bun Shell quoting, escaping, variables, assignments,
export/unset, pipelines, redirects, append, input, logical operators, globbing,
command substitution, cwd, and exit status. Portable builtins SHOULD include
cd, pwd, echo, true, false, env, which, cat, touch, mkdir, rm, mv, ls, dirname,
basename, yes, seq, and exit. RC does not claim Bash compatibility.

Deliberate initial differences are normative: RC Shell is non-interactive and
does not implement Bash job control, functions, aliases, arrays, here-documents,
brace expansion, or Bash control structures. Native interactive work belongs
to `system-login-shell`. Command substitution output must be valid UTF-8 after
bounded capture; ordinary process streams and redirections remain binary-safe.

The upstream baseline is oven-sh/bun commit
ed950b88ab2ec6b58bccdfe7d310731b8ca13c4d from 2026-08-29. Its Rust
src/shell_parser and tests are the extraction source; its Zig/JSC interpreter
is not imported. A stable-Rust parser/AST port and RC-host executor MUST retain
MIT provenance in third_party/bun-shell. RC adapters stay outside vendored
files; source-size exceptions are limited to identified vendored files.

## Trees, streams, terminals, and signals

Every Execution owns one group, including external pipeline children. Unix uses
a session/process group. Windows uses a Job Object with kill-on-close. Node
shutdown, crash, update, kill, or lease expiry MUST leave no descendant.

Nonterminal execution has independent binary stdin/stdout/stderr. PTY/ConPTY
has merged terminal output. EOF and resize are typed. interrupt maps to
Ctrl-C/SIGINT or the best Windows console-group equivalent. terminate maps to
SIGTERM or deterministic graceful Windows termination followed by policy-owned
escalation. kill terminates the group via SIGKILL or Job termination. POSIX
signal numbers MUST NOT cross the contract.

Windows spawning uses Unicode argv/cwd/environment, disciplined handle
inheritance, ConPTY, resize, Job Objects, exit waiting, and cleanup. Unix
preserves exact argv, PTY, session, pipes, waiting, and groups. process-host
MUST NOT parse shell source.

The initial Windows ConPTY client MUST NOT create the requested target before
it belongs to the Execution Job. RC starts a kernel-owned guard blocked on a
private launch gate, assigns that guard to the Job, and only then releases it
to create and wait for the exact requested argv. Assignment or gate failure
MUST kill and reap the guard without starting the target. This closes the
post-CreateProcess descendant escape window while retaining ConPTY ownership.

## Leases and attachments

attached serves browser/human CLI interaction. Losing the final attachment
starts a 60-second reattach grace. Authorized replacement cancels it and
supersedes a stale writer; at most one writer exists. Expiry kills the tree.

managed serves MCP/asynchronous nonterminal work. RPC completion does not end
it. Exit, cancel, max runtime, policy violation, authority expiry or revocation,
Node or machine shutdown, and update do.

scheduled is created through durable scheduler authority and the normal
runtime. It has identical limits and tree ownership. Arbitrary orphan execution
is unsupported. Tests use injectable clocks.

## Registry and output journal

The Node is authoritative for active ID, origin, principal or permit, mode,
native group, lifecycle, stdin and terminal state, attachment, lease, deadline,
and output. Sensitive request fields and transcript bytes stay Node-local or
transit-only.

One binary-safe ordered journal serves reattach and MCP status. It preserves
stdout/stderr boundaries and coalesces only adjacent same-stream chunks; PTY is
stdout. Reads return chunks after an absolute cursor, next cursor,
truncated-before cursor, and whether more remains. Limits roll forward
deterministically. Per-execution limits sit beneath a 256 MiB aggregate Node
journal budget; capacity exhaustion rejects a new Execution and never evicts a
live Execution merely to admit another.

## Browser, CLI, and MCP

HTTP carries allocation metadata and signaling; SSE carries lifecycle metadata;
only encrypted WebRTC DataChannels carry browser/CLI mode, argv or script, cwd,
environment, streams, resize, signal, and attach. No hosted fallback exists.
Session storage MAY hold only a non-sensitive one-time intent. Browser terminal
and rc shell send system-login-shell, never Unix shell source.

MCP keeps process_run, process_status, process_input, and process_cancel. Every
process operation carries both the stable process ID and granted device ID so a
hosted RC restart does not require a server-owned process routing table. The Node
MUST bind an execution to the originating MCP grant ID and MUST reject another
grant even when it belongs to the same user and selects the same device. Run
accepts exactly one nonempty argv or command; command selects rc or system and
defaults to rc. waitSeconds observes only; maxRuntimeSeconds limits lifetime.
MCP remains non-PTY. Input is exact bytes or EOF; valid UTF-8 uses text and
other bytes base64. INT, TERM, and KILL map to typed semantics.

Every Node operation revalidates grant existence, expiry, revocation, selected
device, scope, principal, RC Lock, and process ownership. ID possession is
insufficient. Hosted code may retain bounded correlation channels but MUST NOT
own output. MCP plaintext MUST NOT enter DB, logs, traces, diagnostics, or
history. image_view remains separate.

Nodes advertise execution-v2. Servers MUST NOT send v2 frames to older Nodes
and return Node-upgrade-required. A rolling adapter lasts one release and MUST
NOT preserve two engines.

## Scheduler authority and time

The scheduler imports clock and protected local storage and has no process-host
capability. Its driver reaches execution-runtime through the adapter described
below. Node-local definitions contain ID and name, cron, IANA
timezone, immutable execution/environment/cwd, enabled, overlap, misfire, max
runtime, creator, timestamps, expiry, and permit linkage.

Default overlap forbids overlap and skips while active. Misfire is skip or
bounded run-once; unlimited catch-up is forbidden. Spring-forward missing times
skip; fall-back repeated wall times run at most once. Idempotent occurrence IDs
prevent duplicate fire after restart or clock jumps. No busy loop or
per-schedule OS trigger is allowed.

After each durable tick, the scheduler component returns the next absolute
wake instant. The Node arms one generic timer and schedule management changes
notify that timer immediately. A bounded wake remains active only while a
scheduled execution is running so RC Lock revocation is enforced promptly.

Durable authority is a versioned Owner/passkey-authorized RC Lock permit binding
schedule ID, device, principal, immutable spec hash, cron/timezone policy, max
runtime, and expiry or until-revoked semantics. Revocation prevents future runs
and terminates active scheduled runs. Normal execution policy is rechecked.

`rc schedule add` computes the immutable definition hash locally, publishes only permit metadata,
signs and synchronizes the resulting RC Lock transition, then sends the complete definition to the
Node over encrypted control. `list`, `enable`, and `disable` are Node-local encrypted operations.
`remove` revokes and synchronizes authority before deleting the local definition. Hosted RC MUST
NOT persist cron source, argv, shell source, cwd, or environment values.

The current dynamic service linker cannot transfer a guest-defined resource
between independently instantiated component stores. Scheduler therefore
exports a function-only, ID-addressed driver that durably advances occurrence
state before returning a bounded execution intent. The kernel's mechanism-only
Node adapter submits that intent to the ordinary execution-runtime service; it
MUST NOT translate it into `process-host` directly or make authorization,
misfire, overlap, or lease decisions. This preserves the scheduler → execution
runtime invariant without making guest resource identity a native policy API.

## Platform security, service, and activation

Central helpers own state, data, component, cache, binary, log, service paths,
and executable names; overrides win. Windows defaults use enrolled-user known
folders under LocalAppData and Unix keeps conventional defaults. A nonblocking
state-directory run lock prevents competing Node transport.

Windows sensitive files and directories MUST use DACLs limited to the enrolled
user and required system principals; reads reject materially broad ACLs.
Permission checks MUST NOT be no-ops. Browser opening uses native APIs. The
background Node runs as the enrolled user through Task Scheduler.

Windows native upgrade stages digest and version verified side-by-side
versions, atomically changes launcher activation, health-checks, rolls back,
recovers after interruption, preserves components, denies downgrade, and cleans
later. rc update remains components; rc upgrade remains native. Installers
validate immutable digest, bounds, archive shape, profile lock, activation, and
rollback. Current profile membership comes from its lock; the immutable legacy
bundle retains published members.

The generic component-facing in-place native-replacement host is unavailable
on Windows. Windows callers MUST use `rc upgrade`, which owns the verified
side-by-side activation, rollback journal, and stable launcher transition.

## Retention, failure, and conformance

Hosted process rows contain only required live or bounded lifecycle metadata,
never program, args, script, cwd, environment, streams, transcript, geometry,
or transport details. Diagnostics expose only bounded backend, platform,
counts, and health metadata.

Conformance covers exact argv, environment, binary streams and journal,
PTY/ConPTY, descendants, duplicate and disconnect starts, attachment grace, MCP
isolation, portable shell, fake-clock scheduler and DST, Windows ACL, run lock,
service, and release rollback on native platforms. Linux, macOS, and Windows
MUST use the same WIT contract before legacy deletion. Windows SSH and SFTP
gaps are negotiated honestly and do not block browser, CLI, or MCP parity.
