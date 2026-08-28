#!/usr/bin/env python3
"""Validate declarative RC component profiles."""

from __future__ import annotations

import pathlib
import re
import sys
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent
ID = re.compile(r"^[a-z0-9][a-z0-9:._/-]{0,127}$")
TOKEN = re.compile(r"^[a-z0-9][a-z0-9-]*$")


def read(path: pathlib.Path) -> dict:
    with path.open("rb") as handle:
        return tomllib.load(handle)


def main() -> int:
    components = {
        path.parent.name
        for path in (ROOT / "components").glob("*/component.toml")
    }
    errors: list[str] = []
    manifests = sorted((ROOT / "profiles").glob("*.toml"))
    ids: set[str] = set()
    for path in manifests:
        relative = path.relative_to(ROOT)
        try:
            value = read(path)
        except (OSError, tomllib.TOMLDecodeError) as error:
            errors.append(f"{relative}: {error}")
            continue
        if set(value) != {"schema", "profile"} or value.get("schema") != 1:
            errors.append(f"{relative}: expected schema 1 and one profile table")
            continue
        profile = value.get("profile")
        if not isinstance(profile, dict) or set(profile) != {"id", "components"}:
            errors.append(f"{relative}: profile requires only id and components")
            continue
        profile_id = profile.get("id")
        members = profile.get("components")
        if not isinstance(profile_id, str) or not ID.fullmatch(profile_id):
            errors.append(f"{relative}: invalid profile id")
        elif profile_id in ids:
            errors.append(f"{relative}: duplicate profile id {profile_id}")
        else:
            ids.add(profile_id)
        if not isinstance(members, list) or not members or members != sorted(set(members)):
            errors.append(f"{relative}: components must be a non-empty sorted unique list")
            continue
        for member in members:
            if not isinstance(member, str) or not TOKEN.fullmatch(member):
                errors.append(f"{relative}: invalid component name {member!r}")
            elif member not in components:
                errors.append(f"{relative}: unknown component {member!r}")
    if errors:
        print("profile manifest violations:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    print(f"profiles: ok ({len(manifests)})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
