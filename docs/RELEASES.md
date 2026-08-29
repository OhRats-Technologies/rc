# Releases

RC release tags are immutable.

## Assets

Each release contains four `rc` archives, four kernel archives, and one portable
core component bundle:

```text
rc-darwin-arm64.tar.gz
rc-darwin-amd64.tar.gz
rc-linux-arm64.tar.gz
rc-linux-amd64.tar.gz
rc-kernel-darwin-arm64.tar.gz
rc-kernel-darwin-amd64.tar.gz
rc-kernel-linux-arm64.tar.gz
rc-kernel-linux-amd64.tar.gz
rc-core-components.tar.gz
```

Native archives contain one executable. The core bundle contains `profile.lock`
and the exact core component artifacts. Published assets require SHA-256
digests.

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
4. Build release assets and inspect archive membership.
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

Release workflows build native `rc`, the native kernel, and the portable core
bundle independently. Component implementation changes do not require a native
platform matrix.

## Post-release fixes

Do not rewrite a published tag. Fix forward, validate, and publish a new tag.

## Rollback

Server rollback requires a tested data backup compatible with the selected
image. Node updaters do not perform automatic downgrades; installing an older
native binary is an explicit recovery action.
