#!/usr/bin/env python3
"""Resolve an independently versioned component release from its manifest."""

from __future__ import annotations

import argparse
import json
import pathlib
import tomllib


ROOT = pathlib.Path(__file__).resolve().parents[1]


def components() -> dict[str, dict[str, str]]:
    values: dict[str, dict[str, str]] = {}
    for manifest in sorted((ROOT / "components").glob("*/component.toml")):
        data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        name = manifest.parent.name
        values[name] = {
            "component": name,
            "version": str(data["component"]["version"]),
            "package": str(data["build"]["package"]),
            "artifact": str(data["build"]["artifact"]),
        }
    return values


def main() -> None:
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--component")
    group.add_argument("--tag")
    arguments = parser.parse_args()
    values = components()
    if arguments.component:
        selected = values.get(arguments.component)
    else:
        selected = next(
            (
                value
                for name, value in values.items()
                if arguments.tag == f"{name}-v{value['version']}"
            ),
            None,
        )
    if selected is None:
        parser.error("component or tag does not match a component manifest")
    print(json.dumps(selected, sort_keys=True, separators=(",", ":")))


if __name__ == "__main__":
    main()
