#!/usr/bin/env python3
"""Fail when a repository Markdown link points to a missing local file."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parent.parent
MARKDOWN = [ROOT / "README.md", ROOT / "SECURITY.md", *sorted((ROOT / "docs").glob("*.md"))]
LINK = re.compile(r"(?<!!)\[[^]]+\]\(([^)]+)\)")


def local_target(raw: str) -> str | None:
    target = raw.strip().split(maxsplit=1)[0].strip("<>")
    if not target or target.startswith(("#", "mailto:")) or "://" in target:
        return None
    return unquote(target.split("#", 1)[0])


def main() -> int:
    errors: list[str] = []
    for document in MARKDOWN:
        for raw in LINK.findall(document.read_text(encoding="utf-8")):
            target = local_target(raw)
            if target and not (document.parent / target).resolve().exists():
                errors.append(f"{document.relative_to(ROOT)}: missing link target {raw}")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("documentation links: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
