#!/usr/bin/env python3
"""Keep diagnostics components on metadata-only WIT capabilities."""

from __future__ import annotations

import pathlib
import re
import sys
import tomllib


ROOT = pathlib.Path(__file__).resolve().parents[1]
WIT = "\n".join(
    path.read_text(encoding="utf-8") for path in sorted((ROOT / "wit").glob("*.wit"))
)
ALLOWED = {
    "diagnostics-store": set(),
    "diagnostics-cli": {"ohrats:rc-diagnostics/query@0.1.0"},
    "diagnostics-reporter": {"ohrats:rc-diagnostics/reporting@0.1.0"},
    "diagnostics-ui": {
        "ohrats:rc-diagnostics/query@0.1.0",
        "ohrats:rc-webui/slots@0.1.0",
    },
    "diagnostics-mesh": {
        "ohrats:rc-diagnostics/query@0.1.0",
        "ohrats:rc-mesh-diagnostics/authorization@0.1.0",
    },
    "webui-shell": {"state-store"},
}


def world_body(name: str) -> str:
    match = re.search(rf"world\s+{re.escape(name)}\s*\{{", WIT)
    if not match:
        raise ValueError(f"missing WIT world {name}")
    depth = 1
    position = match.end()
    while position < len(WIT) and depth:
        if WIT[position] == "{":
            depth += 1
        elif WIT[position] == "}":
            depth -= 1
        position += 1
    if depth:
        raise ValueError(f"unterminated WIT world {name}")
    return WIT[match.end() : position - 1]


def imports(name: str) -> set[str]:
    return set(re.findall(r"^\s*import\s+([^;]+);", world_body(name), re.MULTILINE))


def main() -> int:
    errors: list[str] = []
    for component, allowed in ALLOWED.items():
        manifest = ROOT / "components" / component / "component.toml"
        if not manifest.is_file():
            errors.append(f"missing component {component}")
            continue
        with manifest.open("rb") as handle:
            value = tomllib.load(handle)
        world = value["component"]["world"].split("/", 1)[1].split("@", 1)[0]
        actual = imports(world)
        if actual != allowed:
            errors.append(
                f"{component}: imports {sorted(actual)}, expected {sorted(allowed)}"
            )
    if errors:
        print("diagnostics capability violations:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    print("diagnostics capabilities: metadata-only")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
