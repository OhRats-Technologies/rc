#!/usr/bin/env python3
"""Read RC's historical board; live coordination uses scripts/swarm.py."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


def canonical_board() -> Path:
    common = subprocess.check_output(
        ["git", "rev-parse", "--git-common-dir"], text=True
    ).strip()
    common_path = Path(common)
    if not common_path.is_absolute():
        common_path = (Path.cwd() / common_path).resolve()
    else:
        common_path = common_path.resolve()
    return common_path.parent / "BOARD.md"


def read_board(path: Path, tail: int | None) -> None:
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    if tail is not None:
        lines = text.splitlines()
        text = "\n".join(lines[-tail:])
        if text:
            text += "\n"
    sys.stdout.write(text)


def append_message(path: Path, agent: str, kind: str | None, message: str) -> None:
    del path, agent, kind, message
    raise SystemExit(
        "BOARD.md is historical/read-only; use `python3 scripts/swarm.py post`"
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("path", help="print the canonical BOARD.md path")

    read = sub.add_parser("read", help="read the canonical board")
    read.add_argument("--tail", type=int, help="show only the last N lines")

    post = sub.add_parser("post", help="append one dated board message")
    post.add_argument("--agent", required=True, help="worker/agent name")
    post.add_argument("--kind", help="TASK, ACK, CONFLICT, HANDOFF, BLOCKED, rule proposal")
    post.add_argument("message", nargs="*", help="message text; stdin is used when omitted")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    path = canonical_board()
    if args.command == "path":
        print(path)
    elif args.command == "read":
        read_board(path, args.tail)
    elif args.command == "post":
        message = " ".join(args.message) if args.message else sys.stdin.read()
        append_message(path, args.agent, args.kind, message)
        print(path)


if __name__ == "__main__":
    main()
