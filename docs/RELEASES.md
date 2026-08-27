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
- release tag without its leading `v`

Validate them with:

```sh
sh scripts/check-version.sh
sh scripts/check-version.sh 0.16.0-alpha.1
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
   git tag -a v0.16.0-alpha.1 -m 'RC 0.16.0-alpha.1'
   git push origin v0.16.0-alpha.1
   ```

8. Require the `RC release` workflow to pass and publish a non-draft latest release.
9. Download the release archive on each available platform, verify its digest/archive shape, and run `rc version` plus `rc --help`.
10. Test `public/install.sh` and `rc update` against the published release from an isolated temporary home directory.

The workflow verifies formatting, strict Clippy, all Rust targets, dependency audits, tag/version equality, warning-free cross-platform builds, archive names, archive count, and archive contents before publishing. Zig's own deprecated-linker-setting diagnostic is the sole target-toolchain lint suppressed during Linux linking; Rust warnings remain denied.

## Post-release fixes

Never move or overwrite a published tag. Fix forward:

1. Increment the prerelease or patch version in all three version sources.
2. Add the fix to `CHANGELOG.md` and a regression test.
3. Repeat validation and publish a new tag.
4. Mark the new release latest only after its artifacts pass installation and Mac surface smoke tests.

## Rollback

Server rollback requires the matching pre-upgrade database backup and image. Node self-update intentionally refuses semantic-version downgrades. A manual Node rollback is an incident procedure: stop its service, replace the executable with a verified prior artifact, restart, and confirm protocol compatibility.

v0.15 and v0.16 use separate databases and Node state formats. Keep the v0.15 deployment intact until the v0.16 migration is accepted; do not attempt a cross-version database restore.
