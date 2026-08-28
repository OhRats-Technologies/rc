#!/usr/bin/env python3
"""Validate RC component manifests without third-party Python packages."""

from __future__ import annotations

import pathlib
import re
import sys
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent
ID = re.compile(r"^[a-z0-9][a-z0-9:._/-]{0,127}$")
VERSION = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$")
TOKEN = re.compile(r"^[a-z0-9][a-z0-9-]*$")


def read(path: pathlib.Path) -> dict:
    with path.open("rb") as handle:
        return tomllib.load(handle)


def exact_keys(value: dict, expected: set[str], label: str, errors: list[str]) -> None:
    unknown = sorted(set(value) - expected)
    if unknown:
        errors.append(f"{label}: unknown keys {', '.join(unknown)}")


def validate(path: pathlib.Path) -> tuple[list[str], str | None]:
    errors: list[str] = []
    relative = path.relative_to(ROOT)
    try:
        manifest = read(path)
    except (OSError, tomllib.TOMLDecodeError) as error:
        return [f"{relative}: {error}"], None

    exact_keys(manifest, {"schema", "component", "build"}, str(relative), errors)
    if manifest.get("schema") != 1:
        errors.append(f"{relative}: schema must be 1")
    component = manifest.get("component")
    build = manifest.get("build")
    if not isinstance(component, dict) or not isinstance(build, dict):
        errors.append(f"{relative}: component and build tables are required")
        return errors, None

    exact_keys(component, {"id", "version", "world", "wit"}, f"{relative} [component]", errors)
    exact_keys(build, {"kind", "package", "artifact"}, f"{relative} [build]", errors)
    component_id = component.get("id")
    version = component.get("version")
    world = component.get("world")
    wit = component.get("wit")
    package = build.get("package")
    artifact = build.get("artifact")

    if not isinstance(component_id, str) or not ID.fullmatch(component_id):
        errors.append(f"{relative}: invalid component id")
    if not isinstance(version, str) or not VERSION.fullmatch(version):
        errors.append(f"{relative}: invalid semantic version")
    if not isinstance(world, str) or "/" not in world or "@" not in world:
        errors.append(f"{relative}: world must be a versioned WIT world")
    if not isinstance(wit, list) or not wit or len(wit) != len(set(wit)):
        errors.append(f"{relative}: wit must be a non-empty unique list")
    else:
        for name in wit:
            if not isinstance(name, str) or not TOKEN.fullmatch(name):
                errors.append(f"{relative}: invalid WIT package token {name!r}")
            elif not (ROOT / "wit" / f"{name}.wit").is_file():
                errors.append(f"{relative}: missing wit/{name}.wit")

    if build.get("kind") != "rust":
        errors.append(f"{relative}: unsupported build kind")
    if not isinstance(package, str) or not package:
        errors.append(f"{relative}: build.package is required")
    if not isinstance(artifact, str) or not TOKEN.fullmatch(artifact):
        errors.append(f"{relative}: invalid artifact name")

    cargo_path = path.parent / "Cargo.toml"
    if not cargo_path.is_file():
        errors.append(f"{relative}: Rust component is missing Cargo.toml")
    else:
        cargo = read(cargo_path).get("package", {})
        if cargo.get("name") != package:
            errors.append(f"{relative}: build.package does not match Cargo package")
        if cargo.get("version") != version:
            errors.append(f"{relative}: component and Cargo versions differ")
    return errors, artifact if isinstance(artifact, str) else None


def main() -> int:
    manifests = sorted((ROOT / "components").glob("*/component.toml"))
    component_dirs = sorted(path for path in (ROOT / "components").iterdir() if path.is_dir())
    errors: list[str] = []
    artifacts: dict[str, pathlib.Path] = {}
    for directory in component_dirs:
        if not (directory / "component.toml").is_file():
            errors.append(f"{directory.relative_to(ROOT)}: missing component.toml")
    for manifest in manifests:
        manifest_errors, artifact = validate(manifest)
        errors.extend(manifest_errors)
        if artifact:
            previous = artifacts.setdefault(artifact, manifest)
            if previous != manifest:
                errors.append(
                    f"{manifest.relative_to(ROOT)}: artifact {artifact!r} also used by "
                    f"{previous.relative_to(ROOT)}"
                )
    if errors:
        print("component manifest violations:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    print(f"component manifests: ok ({len(manifests)})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
