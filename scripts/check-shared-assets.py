#!/usr/bin/env python3
"""Fail when RC references a shared UI asset that is inconsistent or not published.

Shared visual primitives live in the `ui` repository and are served from
https://assets.ohrats.party/assets/ under content-fingerprinted names such as
`ohrats.abf84a1e6ae1.css`. When `ui` changes, RC must be updated to the new
fingerprints. This check:

1. finds every fingerprinted shared-asset reference tracked in the repository,
2. fails if one asset (e.g. `ohrats.css`) is referenced with several different
   fingerprints (a partially-updated bump), and
3. unless --offline is given, requests each referenced URL and fails on non-200.
"""

from __future__ import annotations

import re
import subprocess
import sys
import urllib.error
import urllib.request
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = "https://assets.ohrats.party/assets/"
REFERENCE = re.compile(r"\b([A-Za-z][A-Za-z0-9_-]*)\.([0-9a-f]{12})\.(css|js|svg|png|woff2)\b")
CONTEXT = re.compile(r"assets\.ohrats\.party|sharedVisualAssets|SHARED_BASE|\bbase\s*\+|/assets/")
SKIP_SUFFIXES = {".lock", ".sum", ".wasm", ".png", ".woff2", ".gz", ".zip"}


def tracked_files() -> list[Path]:
    out = subprocess.run(
        ["git", "ls-files", "-z"], cwd=ROOT, check=True, capture_output=True
    ).stdout.decode()
    return [ROOT / name for name in out.split("\0") if name and (ROOT / name).suffix not in SKIP_SUFFIXES]


def collect() -> dict[str, dict[str, set[str]]]:
    found: dict[str, dict[str, set[str]]] = defaultdict(lambda: defaultdict(set))
    for path in tracked_files():
        try:
            text = path.read_text(encoding="utf-8")
        except (UnicodeDecodeError, FileNotFoundError):
            continue
        if not CONTEXT.search(text):
            continue
        for match in REFERENCE.finditer(text):
            stem, digest, ext = match.groups()
            found[f"{stem}.{ext}"][digest].add(str(path.relative_to(ROOT)))
    return found


def exists(url: str) -> bool:
    request = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "rc-asset-check"})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return response.status == 200
    except urllib.error.HTTPError as error:
        return error.code == 200
    except urllib.error.URLError as error:
        print(f"error: cannot reach {url}: {error.reason}", file=sys.stderr)
        raise SystemExit(2)


def main() -> int:
    offline = "--offline" in sys.argv[1:]
    found = collect()
    failures = 0
    for name, digests in sorted(found.items()):
        if len(digests) > 1:
            failures += 1
            print(f"error: {name} is referenced with several fingerprints:", file=sys.stderr)
            for digest, files in sorted(digests.items()):
                print(f"  {digest}: {len(files)} file(s), e.g. {sorted(files)[0]}", file=sys.stderr)
    if not offline:
        for name, digests in sorted(found.items()):
            for digest in sorted(digests):
                stem, ext = name.rsplit(".", 1)
                url = f"{BASE}{stem}.{digest}.{ext}"
                if not exists(url):
                    failures += 1
                    print(f"error: {url} is not published (referenced by {sorted(digests[digest])[0]})", file=sys.stderr)
    if failures:
        print("Update RC to the current fingerprints from the `ui` build output.", file=sys.stderr)
        return 1
    print(f"shared UI assets OK ({sum(len(d) for d in found.values())} fingerprint(s), {len(found)} asset(s))")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
