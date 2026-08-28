#!/usr/bin/env python3
"""Create a deterministic, zero-compile profile assembly plan."""

from __future__ import annotations

import argparse
import json
import pathlib
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent


def read(path: pathlib.Path) -> dict:
    with path.open("rb") as handle:
        return tomllib.load(handle)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("profile")
    arguments = parser.parse_args()
    profile_path = ROOT / "profiles" / f"{arguments.profile}.toml"
    if not profile_path.is_file():
        parser.error(f"unknown profile {arguments.profile!r}")
    profile = read(profile_path)["profile"]
    components = []
    for name in profile["components"]:
        manifest = read(ROOT / "components" / name / "component.toml")
        component = manifest["component"]
        build = manifest["build"]
        components.append(
            {
                "artifact": build["artifact"],
                "id": component["id"],
                "source": f"workspace:components/{name}",
                "version": component["version"],
                "world": component["world"],
            }
        )
    plan = {
        "schema": 1,
        "profile": profile["id"],
        "components": components,
    }
    output = ROOT / "dist" / "profiles" / f"{arguments.profile}.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(
        json.dumps(plan, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(output.relative_to(ROOT))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
