# Installation

The public installer is POSIX `sh`. It downloads the platform `rc` archive,
matching kernel archive, and the core component asset from the immutable GitHub
release returned by the release API. GitHub SHA-256 digests are required and
verified before any archive is read or activated.

Current releases publish `rc-core-profile.tar.gz`. It contains exactly
`profile.lock` plus the twelve files under `components/` in the core profile.
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

Activation uses same-filesystem temporary files and retains the previous native
pair and installer-owned core component files under
`~/.local/share/rc/rollback/previous`. `.core` markers are distinct from package
manager `.managed` markers. A component without a matching `.core` marker is a
local override and is not replaced by the installer.

Run the deterministic installer smoke with:

```sh
sh scripts/smoke-install.sh
```

Build release core assets with:

```sh
packaging/build-core-bundle.sh
packaging/build-legacy-core-bundle.sh
```
