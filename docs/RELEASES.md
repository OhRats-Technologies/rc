# Releases

RC release tags are immutable.

## Assets

Each release contains five native `rc` archives, the current core profile, and
the legacy upgrade bridge:

```text
rc-darwin-arm64.tar.gz
rc-darwin-amd64.tar.gz
rc-linux-arm64.tar.gz
rc-linux-amd64.tar.gz
rc-windows-amd64.tar.gz
rc-core-profile.tar.gz
rc-core-components.tar.gz
rc-upgrade.json
install.sh
install.ps1
```

Every native archive contains one executable: `rc`, or `rc.exe` on Windows.
`rc-core-profile.tar.gz` contains `profile.lock` and the exact components
selected by `profiles/core.toml`. `rc-core-components.tar.gz` contains the ten
names required by the immutable v0.19.2 updater and no lock. Published assets
require GitHub SHA-256 digests.

There is no separate `rc-kernel` asset. The Go `rc` binary embeds the Wasmtime
host; portable components remain independent artifacts.

The installers target the single `rc` archive and the core profile bundle. The
release workflow builds them as assets and runs installation smoke on Linux,
macOS, and Windows after publishing.

## Version

The release version is validated by:

```sh
VERSION="$(sh scripts/check-version.sh)"
sh scripts/check-version.sh "$VERSION"
```

`pkg/version/version.go`, `package.json`, and the newest changelog entry must
agree with the tag.

## Pre-release checks

1. Run the validation commands in [Development](DEVELOPMENT.md#validation).
2. Run dependency audits (`govulncheck ./...`, `bun audit`).
3. Build and smoke-test the production image (`scripts/smoke-container.sh`).
4. Build release assets and inspect both core archive formats.
5. Exercise browser setup/login, Node enrollment, direct control, CLI, and MCP
   on representative systems once those behaviors exist.
6. Confirm `git status --short` is empty.
7. Push `main` and require CI success, including the macOS and Windows jobs.

## Tag

```sh
VERSION="$(sh scripts/check-version.sh)"
git tag -a "v$VERSION" -m "RC $VERSION"
git push origin "v$VERSION"
```

`release-prebuild.yml` builds native `rc` archives with cgo on Linux, macOS, and
Windows. `core-components.yml` builds portable components independently.
Component implementation changes do not require a native platform matrix unless
a native dependency changed.

## Post-release fixes

Do not rewrite a published tag. Fix forward, validate, and publish a new tag.

## Rollback

Server rollback requires a tested data backup compatible with the selected
image. Node updaters do not perform automatic downgrades; installing an older
native binary is an explicit recovery action.
