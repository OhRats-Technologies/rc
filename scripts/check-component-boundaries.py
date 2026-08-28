#!/usr/bin/env python3
"""Validate native-kernel and component build isolation."""

from __future__ import annotations

import pathlib
import sys
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent


def load(path: pathlib.Path) -> dict:
    with path.open("rb") as handle:
        return tomllib.load(handle)


def local_paths(manifest: pathlib.Path, data: dict) -> list[pathlib.Path]:
    paths: list[pathlib.Path] = []
    sections = [data.get("dependencies", {}), data.get("dev-dependencies", {})]
    sections.extend(data.get("target", {}).values())
    for section in sections:
        dependencies = section
        if "dependencies" in section:
            dependencies = section["dependencies"]
        for value in dependencies.values():
            if isinstance(value, dict) and "path" in value:
                paths.append((manifest.parent / value["path"]).resolve())
    return paths


def main() -> int:
    errors: list[str] = []
    kernel = ROOT / "kernel" / "Cargo.toml"
    manifests = [kernel, *sorted((ROOT / "components").glob("*/Cargo.toml"))]
    for manifest in manifests:
        data = load(manifest)
        relative = manifest.relative_to(ROOT)
        if "workspace" not in data:
            errors.append(f"{relative}: must own an independent Cargo workspace")
        if not (manifest.parent / "Cargo.lock").is_file():
            errors.append(f"{relative}: missing independent Cargo.lock")
        if manifest != kernel and not (manifest.parent / "component.toml").is_file():
            errors.append(f"{relative}: missing component.toml")
        for dependency in local_paths(manifest, data):
            if manifest == kernel and ROOT / "crates" in dependency.parents:
                errors.append(f"{relative}: kernel depends on legacy crate {dependency}")
            if manifest != kernel and dependency == (ROOT / "kernel").resolve():
                errors.append(f"{relative}: component depends on native kernel")
            if manifest != kernel and ROOT / "crates" in dependency.parents:
                errors.append(f"{relative}: component depends on legacy crate {dependency}")

    root_workspace = load(ROOT / "Cargo.toml").get("workspace", {})
    for member in root_workspace.get("members", []):
        if member == "kernel" or member.startswith("components/"):
            errors.append(f"Cargo.toml: {member} must not join the legacy workspace")

    wit_files = sorted((ROOT / "wit").glob("*.wit")) + sorted(
        (ROOT / "wit" / "deps").glob("*/*.wit")
    )
    if not wit_files:
        errors.append("wit/: at least one WIT package is required")
    for path in wit_files:
        first = path.read_text(encoding="utf-8").lstrip().splitlines()[0]
        if not first.startswith("package ohrats:"):
            errors.append(f"{path.relative_to(ROOT)}: package must use ohrats namespace")

    if errors:
        print("component boundary violations:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    print(f"component boundaries: ok ({len(manifests) - 1} components)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
