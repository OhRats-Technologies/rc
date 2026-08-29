# Installation

The public installer is a POSIX `sh` script. It downloads the platform RC
archive, matching kernel archive, and `rc-core-components.tar.gz` from the
immutable GitHub release named by the release API response. Every asset must
provide a GitHub SHA-256 digest; the installer verifies the digest before it
reads or activates an archive.

The core archive is a bounded tar file containing exactly `profile.lock` and
the ten files under `components/` in the core profile. The lock format is
deliberately line-oriented so validation does not require Python, jq, or RC:

```text
schema 1
profile ohrats:core
component <name> sha256:<64 hex characters>
```

The installer verifies every component digest and asks the staged kernel to
repair the staged graph before activation. It places `rc` and `rc-kernel` in
`~/.local/bin` and components in `~/.local/share/rc/components` by default.
`RC_INSTALL_BIN_DIR`, `RC_DATA_DIR`, and `RC_COMPONENT_DIR` provide explicit
test or packaging paths. `RC_STATE_DIR` controls where enrollment state is
read when deciding whether to install the background service.

Activation uses same-filesystem temporary files and retains the prior native
pair and core component files under `~/.local/share/rc/rollback/previous`.
The `.core` files are installer-owned markers, deliberately distinct from the
package manager's `.managed` markers. This keeps the native/core bundle under
installer ownership: `rc update` does not claim, remove, or reconcile these
files, while a matching `.core` marker lets a later installer replace its own
artifact. Locally overridden components (missing a matching `.core` marker) are
left untouched. Enrollment and service setup happen only after the verified
runtime is active; installation never invokes `rc upgrade` as a second step.

Run the deterministic fixture smoke test with:

```sh
sh scripts/smoke-install.sh
```

Release jobs should create the core archive with:

```sh
packaging/build-core-bundle.sh
```
