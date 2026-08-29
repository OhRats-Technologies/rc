#!/usr/bin/env python3
"""Resolve build units affected by a Git diff."""

from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent
ALL_ZERO = "0" * 40
KERNEL_WIT = {"http", "plugin", "process", "storage", "transport"}


def component_metadata() -> dict[str, set[str]]:
    values: dict[str, set[str]] = {}
    for manifest in sorted((ROOT / "components").glob("*/component.toml")):
        with manifest.open("rb") as handle:
            data = tomllib.load(handle)
        values[manifest.parent.name] = set(data["component"]["wit"])
    return values


def profile_metadata() -> dict[str, set[str]]:
    values: dict[str, set[str]] = {}
    for manifest in sorted((ROOT / "profiles").glob("*.toml")):
        with manifest.open("rb") as handle:
            data = tomllib.load(handle)
        values[manifest.stem] = set(data["profile"]["components"])
    return values


def changed_paths(arguments: argparse.Namespace) -> list[str]:
    if arguments.all:
        return ["<all>"]
    if arguments.paths:
        return arguments.paths
    base = arguments.base
    if not base or base == ALL_ZERO:
        return ["<all>"]
    command = ["git", "diff", "--name-only", base, arguments.head]
    result = subprocess.run(command, cwd=ROOT, check=True, text=True, capture_output=True)
    return [line for line in result.stdout.splitlines() if line]


def resolve(paths: list[str]) -> dict:
    metadata = component_metadata()
    profiles = profile_metadata()
    all_components = set(metadata)
    components: set[str] = set()
    affected_profiles: set[str] = set()
    flags = {
        "kernel": False,
        "legacy_rust": False,
        "web": False,
        "image": False,
        "docs": False,
        "legacy_security": False,
    }
    if "<all>" in paths:
        components = all_components
        affected_profiles = set(profiles)
        flags = {name: True for name in flags}
    else:
        for raw in paths:
            path = pathlib.PurePosixPath(raw)
            parts = path.parts
            if not parts:
                continue
            root = parts[0]
            if root == "kernel":
                flags["kernel"] = True
            elif root == "wit":
                if len(parts) == 2:
                    flags["kernel"] = True
                    package = path.stem
                elif len(parts) >= 4 and parts[1] == "deps":
                    package = parts[2]
                    if package in KERNEL_WIT:
                        flags["kernel"] = True
                else:
                    package = path.stem
                components.update(
                    name for name, dependencies in metadata.items() if package in dependencies
                )
            elif root == "components" and len(parts) > 1:
                if parts[1] in metadata:
                    components.add(parts[1])
            elif raw in COMPONENT_TOOLING:
                components.update(all_components)
            elif raw in RUNTIME_TOOLING:
                flags["kernel"] = True
                components.update(RUNTIME_TOOLING[raw])
            elif root == "profiles":
                if path.stem in profiles:
                    affected_profiles.add(path.stem)
                else:
                    affected_profiles.update(profiles)
            elif raw in PROFILE_TOOLING:
                affected_profiles.update(profiles)
            elif root in {"crates", "fixtures"} or raw in RUST_ROOTS:
                flags["legacy_rust"] = True
                flags["image"] = True
                if raw.endswith(("Cargo.lock", "Cargo.toml")):
                    flags["legacy_security"] = True
            elif root == "web" or raw in WEB_ROOTS:
                flags["web"] = True
                flags["image"] = True
                if raw in {"package.json", "bun.lock"}:
                    flags["legacy_security"] = True
            elif root in {"docker", "public"} or raw in IMAGE_ROOTS:
                flags["image"] = True
            elif root == "docs" or raw in DOC_ROOTS:
                flags["docs"] = True
            elif root == "scripts" and raw not in STATIC_ONLY:
                flags["legacy_rust"] = True
                flags["web"] = True
                flags["image"] = True

    affected_profiles.update(
        name
        for name, members in profiles.items()
        if members.intersection(components)
    )
    names = sorted(components)
    profile_names = sorted(affected_profiles)
    result = {
        **flags,
        "components": names,
        "component_matrix": {"include": [{"name": name} for name in names]},
        "profiles": profile_names,
        "profile_matrix": {"include": [{"name": name} for name in profile_names]},
    }
    return result


COMPONENT_TOOLING = {
    "schemas/component.schema.json",
    "scripts/build-component.sh",
    "scripts/check-component.sh",
    "scripts/check-components.sh",
    "scripts/validate-components.py",
}
RUNTIME_TOOLING = {
    "scripts/smoke-api-credentials.sh": {
        "api-credential-fixture",
        "api-credential-store",
        "identity-store",
        "webauthn-es256",
    },
    "scripts/smoke-authenticated-webui.sh": {
        "identity-fixture",
        "identity-store",
        "webauthn-es256",
        "webui-app",
        "webui-shell",
    },
    "scripts/smoke-diagnostics.sh": {
        "diagnostics-cli",
        "diagnostics-mesh",
        "diagnostics-reporter",
        "diagnostics-store",
        "diagnostics-ui",
        "webui-shell",
    },
    "scripts/smoke-kernel.sh": {
        "call-context-consumer",
        "fixture-broken",
        "fixture-consumer",
        "fixture-provider",
        "fixture-provider-v2",
    },
    "scripts/smoke-node-components.sh": {
        "process-policy",
        "transport-test",
        "transport-webrtc",
    },
    "scripts/smoke-identity.sh": {
        "identity-fixture",
        "identity-store",
        "webauthn-es256",
    },
    "scripts/smoke-packages.sh": {
        "http-source",
        "local-source",
        "oci-source",
        "package-manager",
    },
    "scripts/smoke-storage.sh": {"storage-fixture"},
    "scripts/smoke-webauthn.sh": {"webauthn-es256", "webauthn-fixture"},
    "scripts/smoke-web-runtime.sh": {
        "diagnostics-store",
        "diagnostics-ui",
        "webui-shell",
    },
}
PROFILE_TOOLING = {
    "schemas/profile.schema.json",
    "scripts/assemble-profile.py",
    "scripts/validate-profiles.py",
}
RUST_ROOTS = {"Cargo.toml", "Cargo.lock", "rust-toolchain.toml"}
WEB_ROOTS = {"package.json", "bun.lock", "tsconfig.json"}
IMAGE_ROOTS = {"Dockerfile", ".dockerignore", "docker-entrypoint.sh"}
DOC_ROOTS = {"README.md", "SECURITY.md", "ROADMAP.md", "CHECKLIST.md", "AGENTS.md"}
STATIC_ONLY = {
    "scripts/affected-units.py",
    "scripts/check-component-boundaries.py",
    "scripts/check-doc-links.py",
    "scripts/check-source-size.sh",
    "scripts/test-affected-units.py",
    "scripts/validate-profiles.py",
}


def write_github(path: pathlib.Path, result: dict) -> None:
    with path.open("a", encoding="utf-8") as output:
        for name, value in result.items():
            if isinstance(value, bool):
                encoded = str(value).lower()
            else:
                encoded = json.dumps(value, separators=(",", ":"))
            output.write(f"{name}={encoded}\n")


def parse() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base")
    parser.add_argument("--head", default="HEAD")
    parser.add_argument("--all", action="store_true")
    parser.add_argument("--paths", nargs="*")
    parser.add_argument("--github-output", type=pathlib.Path)
    return parser.parse_args()


def main() -> int:
    arguments = parse()
    result = resolve(changed_paths(arguments))
    if arguments.github_output:
        write_github(arguments.github_output, result)
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
