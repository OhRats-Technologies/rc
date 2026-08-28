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

1. Finish code, documentation, and `CHANGELOG.md`.
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

8. Require the `RC release` workflow to pass and publish a non-draft latest release. Release artifacts must come from the exact tagged SHA and pass archive validation before publication.
9. Download the release archive on each available platform, verify its digest/archive shape, and run `rc version` plus `rc --help`.
10. Test `public/install.sh` and `rc update` against the published release from an isolated temporary home directory.

`CI` verifies formatting, strict Clippy, Rust tests, dependency audits, source size, documentation links, browser checks, shell/workflow linting, and the production container. Cross-platform release artifacts are built from the same source revision. Rust warnings remain denied.

Darwin arm64 and amd64 archives are built on Apple Silicon runners. Linux archives use `cargo-zigbuild`. The production Dockerfile uses a pinned `cargo-chef` builder so dependency compilation is reusable across source-only builds.

## Post-release fixes

Never move or overwrite a published tag. Fix forward:

1. Increment the prerelease or patch version in all three version sources.
2. Add the fix to `CHANGELOG.md` and a regression test.
3. Repeat validation and publish a new tag.
4. Mark the new release latest only after its artifacts pass installation and Mac surface smoke tests.

## Rollback

Server rollback requires a database backup created by the matching server build and the corresponding image. Node self-update refuses semantic-version downgrades. A manual Node rollback is an incident procedure: stop its service, replace the executable with a verified prior artifact, restart, and verify connectivity.
