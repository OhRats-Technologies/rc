# Releases

RC release tags are immutable.

## Assets

Each release contains five `rc` archives, five kernel archives, the current core
profile, the legacy upgrade bridge, and the verified Windows installer:

```text
rc-darwin-arm64.tar.gz
rc-darwin-amd64.tar.gz
rc-linux-arm64.tar.gz
rc-linux-amd64.tar.gz
rc-windows-amd64.tar.gz
rc-kernel-darwin-arm64.tar.gz
rc-kernel-darwin-amd64.tar.gz
rc-kernel-linux-arm64.tar.gz
rc-kernel-linux-amd64.tar.gz
rc-kernel-windows-amd64.tar.gz
rc-core-profile.tar.gz
rc-core-components.tar.gz
install.ps1
```

Native archives contain one executable. `rc-core-profile.tar.gz` contains
`profile.lock` and the exact components selected by `profiles/core.toml`.
`rc-core-components.tar.gz`
contains the ten names required by the immutable v0.19.2 updater and no lock.
Published assets require GitHub SHA-256 digests.
Windows activation retains active and previous native generations, removes
older versioned generations after commit, and rejects both CLI and independent
kernel downgrades.

Current installers and updaters prefer the profile asset. The legacy asset
exists only so v0.19.2 can reach a newer native platform; a subsequent
same-version `rc upgrade` repairs an incomplete legacy core using the profile
asset.

## Version

The release version is validated by:

```sh
VERSION="$(sh scripts/check-version.sh)"
sh scripts/check-version.sh "$VERSION"
```

Versioned release metadata and the newest changelog entry must agree with the
tag.

## Pre-release checks

1. Run the validation commands in [Development](DEVELOPMENT.md#validation).
2. Run dependency audits.
3. Build and smoke-test the production image.
4. Build release assets and inspect both core archive formats.
5. Exercise browser setup/login, Node enrollment, direct control, CLI, MCP, and
   SSH on representative systems.
6. Confirm `git status --short` is empty.
7. Push `main` and require CI success.

## Tag

```sh
VERSION="$(sh scripts/check-version.sh)"
git tag -a "v$VERSION" -m "RC $VERSION"
git push origin "v$VERSION"
```

Release workflows build native `rc`, the native kernel, and portable component
assets independently. Component implementation changes do not require a native
platform matrix unless a native dependency changed.

## Post-release fixes

Do not rewrite a published tag. Fix forward, validate, and publish a new tag.

## Rollback

Server rollback requires a tested data backup compatible with the selected
image. Node updaters do not perform automatic downgrades; installing an older
native binary is an explicit recovery action.
