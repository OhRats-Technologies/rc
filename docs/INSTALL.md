# Installation

The public installers are POSIX `install.sh` and Windows PowerShell
`install.ps1`. They download the platform `rc` archive,
matching kernel archive, and the core component asset from the immutable GitHub
release returned by the release API. GitHub SHA-256 digests are required and
verified before any archive is read or activated.

The canonical core profile is an atomic part of the native runtime. Install and
upgrade replace its named members together while leaving non-core third-party
components untouched. RC performs a semantic policy/runtime check before native
activation; file integrity alone is not considered healthy.

Running the installer again is safe and updates the existing installation. If
an enrollment token is supplied while the default state directory already has
a device identity, the installer leaves that identity unchanged and does not
consume the token. Use the separately generated `rc enroll` command on an
unenrolled machine. RC supports one default per-user background enrollment;
advanced foreground Nodes may use distinct explicit `--state-dir` directories.

Current releases publish `rc-core-profile.tar.gz`. It contains exactly
`profile.lock` plus the exact files declared by the core profile.
The lock is line-oriented:

```text
schema 1
profile ohrats:core
component <name> sha256:<64 hex characters>
```

The installer verifies every locked component digest and asks the staged kernel
to validate the staged graph before activation.

`rc-core-components.tar.gz` is the ten-component compatibility asset accepted
by the released v0.19.2 upgrader. It has no `profile.lock`. The installer uses
it only when a release does not provide the current profile asset. This keeps
old immutable releases installable without weakening validation of current
releases.

By default, `rc` and `rc-kernel` are installed in `~/.local/bin`; components go
in `~/.local/share/rc/components`. `RC_INSTALL_BIN_DIR`, `RC_DATA_DIR`,
`RC_COMPONENT_DIR`, and `RC_STATE_DIR` provide explicit test or packaging
locations.

On Windows, run the signed release copy of `install.ps1` as the enrolled user.
Defaults live under `%LOCALAPPDATA%\OhRats\RC`. The installer stages the native
pair under `data\runtime\versions\VERSION`, validates it with the staged core
profile, and atomically changes `data\runtime\active`. The controller remains
at `bin\rc.exe`; service registration resolves the versioned active kernel.
Installer-owned runtime, component, state, rollback, binary, and staging
directories receive protected DACLs for the enrolled user, SYSTEM, and local
Administrators before artifacts are written.
The previous activation pointer, stable controller, and managed core component
set are retained for interruption recovery. Activation writes a durable
journal only after that rollback snapshot is complete; a later installer or
`rc upgrade` restores it before new work if interruption left the journal.
If the running stable Windows controller itself must be rolled back, RC stages
a post-exit helper, exits with a rerun instruction, and leaves the journal until
that helper commits the replacement.
When `rc upgrade` is itself running from `bin\rc.exe`, a staged external
PowerShell helper waits for that process to exit before replacing the stable
controller image; RC never renames over its running Windows executable.
The active and previous runtime generations remain available for rollback;
after activation commits, RC removes only older semantic-version directories.
CLI and independently versioned kernel downgrades are rejected separately.

The normal Windows Node is a per-user interactive Task Scheduler task. It
starts at that user's logon and runs only while an interactive user session is
available; it is not an unattended machine service and never runs as
LocalSystem. A future unattended service mode requires a separately reviewed
identity and filesystem-authority model.

Activation uses same-filesystem temporary files and retains the previous native
pair and installer-owned core component files under
`~/.local/share/rc/rollback/previous`. `.core` markers are distinct from package
manager `.managed` markers. A component without a matching `.core` marker is a
local override and is not replaced by the installer.

Run the deterministic installer smoke with:

```sh
sh scripts/smoke-install.sh
```

PowerShell syntax and native Windows runtime integration are gated in
`windows-latest` CI.

Build release core assets with:

```sh
packaging/build-core-bundle.sh
packaging/build-legacy-core-bundle.sh
```
