# Releases

RC releases are immutable GitHub tags with four CLI/Node archives:

```text
rc-darwin-arm64.tar.gz
rc-darwin-amd64.tar.gz
rc-linux-arm64.tar.gz
rc-linux-amd64.tar.gz
```

Each archive must contain exactly one executable named `rc`.

## Version sources

These values must match:

- `package.json`
- `crates/rc-cli/Cargo.toml`
- `crates/rc-server/Cargo.toml`
- the newest `CHANGELOG.md` release heading
- release tag without its leading `v`

Validate them with:

```sh
VERSION="$(sh scripts/check-version.sh)"
sh scripts/check-version.sh "$VERSION"
```

## Release checklist

1. Finish code, documentation, compatibility notes, and `CHANGELOG.md`.
2. Run every command in [Development](DEVELOPMENT.md#required-validation), including RustSec and Bun dependency audits.
3. Build and inspect the production Docker image.
4. Exercise public pages, setup authorization, authenticated page integration tests, Node enrollment, direct control, CLI help/status, and service definitions on macOS.
5. Confirm `git status --short` is empty.
6. Push `main` and require the CI workflow to pass.
7. Create and push an annotated or signed tag:

   ```sh
   VERSION="$(sh scripts/check-version.sh)"
   git tag -a "v$VERSION" -m "RC $VERSION"
   git push origin "v$VERSION"
   ```

8. Require the `RC release` workflow to pass and publish a non-draft latest release. The tag workflow intentionally does not rerun the full integration suite; it verifies the tag/version, requires the tagged commit to be on `main`, then builds and validates the four release archives.
9. Download the release archive on each available platform, verify its digest/archive shape, and run `rc version` plus `rc --help`.
10. Test `public/install.sh` and `rc update` against the published release from an isolated temporary home directory.

The `CI` workflow on `main` verifies formatting, strict Clippy, all Rust targets, dependency audits, source size, documentation links, browser type/build checks, shell/workflow linting, and the production container smoke test. The `RC release` workflow avoids duplicating those expensive checks: it verifies tag/version equality and `main` ancestry, then performs warning-free cross-platform builds and validates archive names, count, and contents before publishing. Zig's own deprecated-linker-setting diagnostic is the sole target-toolchain lint suppressed during Linux linking; Rust warnings remain denied.

Both workflows cache Rust dependency/build state. The CI image job also uses the GitHub Actions BuildKit cache, and Linux release jobs cache the `cargo-zigbuild` binary. These caches are performance optimizations only; cache misses must still produce the same validated artifacts.

## Post-release fixes

Never move or overwrite a published tag. Fix forward:

1. Increment the prerelease or patch version in all three version sources.
2. Add the fix to `CHANGELOG.md` and a regression test.
3. Repeat validation and publish a new tag.
4. Mark the new release latest only after its artifacts pass installation and Mac surface smoke tests.

## Rollback

Server rollback requires the matching pre-upgrade database backup and image. Node self-update intentionally refuses semantic-version downgrades. A manual Node rollback is an incident procedure: stop its service, replace the executable with a verified prior artifact, restart, and confirm protocol compatibility.

v0.15 and v0.16 use separate databases and Node state formats. Keep the v0.15 deployment intact until the v0.16 migration is accepted; do not attempt a cross-version database restore.
