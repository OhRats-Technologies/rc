#!/usr/bin/env python3
"""Validate kernel/component build isolation for the Go runtime."""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
ROOT_MODULE = "github.com/OhRats-Technologies/rc"
COMPONENT_MODULE = "wit_component"


def go_mod_requirements(path: pathlib.Path) -> list[str]:
    text = path.read_text(encoding="utf-8")
    values: list[str] = []
    for match in re.finditer(r"(?ms)^require\s*\((.*?)^\)", text):
        values.extend(line.split()[0] for line in match.group(1).splitlines() if line.strip())
    for match in re.finditer(r"(?m)^require\s+(\S+)", text):
        values.append(match.group(1))
    return values


def go_mod_replaces(path: pathlib.Path) -> list[str]:
    text = path.read_text(encoding="utf-8")
    values: list[str] = []
    for match in re.finditer(r"(?ms)^replace\s*\((.*?)^\)", text):
        values.extend(line.split()[0] for line in match.group(1).splitlines() if line.strip())
    for match in re.finditer(r"(?m)^replace\s+(\S+)", text):
        values.append(match.group(1))
    return values


def go_imports(path: pathlib.Path) -> set[str]:
    text = path.read_text(encoding="utf-8")
    imports: set[str] = set()
    for match in re.finditer(r'(?m)^\s*(?:[A-Za-z0-9_.]+\s+)?"([^"]+)"\s*$', text):
        imports.add(match.group(1))
    return imports


def main() -> int:
    errors: list[str] = []
    legacy = [ROOT / "crates", ROOT / "kernel"]
    for directory in legacy:
        if directory.exists():
            errors.append(f"{directory.name}/: legacy Rust runtime must not remain")

    root_go_mod = ROOT / "go.mod"
    if not root_go_mod.is_file():
        errors.append("go.mod: the native kernel module is required")
        root_requirements: list[str] = []
    else:
        root_requirements = go_mod_requirements(root_go_mod)
        for requirement in root_requirements:
            if requirement.startswith(ROOT_MODULE + "/"):
                errors.append(f"go.mod: kernel must not require component module {requirement}")

    manifests = sorted((ROOT / "components").glob("*/component.toml"))
    if not manifests:
        errors.append("components/: at least one component manifest is required")
    for manifest in manifests:
        directory = manifest.parent
        relative = directory.relative_to(ROOT)
        go_mod = directory / "go.mod"
        if not go_mod.is_file():
            errors.append(f"{relative}: must own an independent go.mod")
            continue
        text = go_mod.read_text(encoding="utf-8")
        if not re.search(r"(?m)^module\s+wit_component\s*$", text):
            errors.append(f"{relative}: go.mod must declare module {COMPONENT_MODULE}")
        if (directory / "Cargo.toml").is_file():
            errors.append(f"{relative}: remove the legacy Cargo manifest")
        for requirement in go_mod_requirements(go_mod) + go_mod_replaces(go_mod):
            if requirement.startswith(ROOT_MODULE):
                errors.append(f"{relative}: component must not depend on the native runtime")
        for source in directory.rglob("*.go"):
            for imported in go_imports(source):
                if imported.startswith(ROOT_MODULE):
                    errors.append(
                        f"{source.relative_to(ROOT)}: component imports native runtime package {imported}"
                    )

    go_work = ROOT / "go.work"
    if go_work.is_file():
        for line in go_work.read_text(encoding="utf-8").splitlines():
            entry = line.strip()
            if entry.startswith("./components/") or entry == "./components":
                errors.append("go.work: components must not join the native workspace")

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
    print(f"component boundaries: ok ({len(manifests)} components)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
