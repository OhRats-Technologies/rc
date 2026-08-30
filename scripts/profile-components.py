#!/usr/bin/env python3
"""Print a profile's component names in declared order."""

from __future__ import annotations

import pathlib
import sys
import tomllib


ROOT = pathlib.Path(__file__).resolve().parent.parent


def main() -> int:
    if len(sys.argv) != 2 or not sys.argv[1].replace("-", "").isalnum():
        raise SystemExit("usage: profile-components.py PROFILE")
    path = ROOT / "profiles" / f"{sys.argv[1]}.toml"
    with path.open("rb") as handle:
        profile = tomllib.load(handle)["profile"]
    for component in profile["components"]:
        print(component)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
