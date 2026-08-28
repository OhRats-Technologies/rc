#!/usr/bin/env python3
"""Read and append RC swarm coordination state."""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote


TOKEN = re.compile(r"^[a-z0-9][a-z0-9._-]{1,63}$")
KINDS = {
    "CLM": ("c", "CLAIM"),
    "ACK": ("a", "ACK"),
    "PRP": ("p", "PROPOSAL"),
    "QRY": ("q", "QUESTION"),
    "RSP": ("r", "RESPONSE"),
    "STA": ("s", "STATUS"),
    "BLK": ("b", "BLOCKED"),
    "CFT": ("x", "CONFLICT"),
    "WRN": ("w", "WARNING"),
    "TST": ("t", "TEST"),
    "PUB": ("u", "PUBLISHED"),
    "HOF": ("h", "HANDOFF"),
    "RES": ("d", "RESOLVED"),
    "CAN": ("z", "CANCELLED"),
}
ALIASES = {code: code for code in KINDS} | {
    label: code for code, (_, label) in KINDS.items()
} | {
    "TASK": "CLM",
    "CLAIM": "CLM",
    "HANDOFF": "HOF",
    "BLOCKER": "BLK",
}
CHAR_KIND = {char: (code, label) for code, (char, label) in KINDS.items()}


def common_dir() -> Path:
    value = subprocess.check_output(
        ["git", "rev-parse", "--git-common-dir"], text=True
    ).strip()
    path = Path(value)
    return path.resolve() if path.is_absolute() else (Path.cwd() / path).resolve()


def repository_id() -> str:
    common = common_dir()
    name = re.sub(r"[^a-z0-9._-]+", "-", common.parent.name.lower()).strip("-") or "repo"
    digest = hashlib.sha256(str(common).encode()).hexdigest()[:12]
    return f"{name}-{digest}"


def swarm_root() -> Path:
    override = os.environ.get("RC_SWARM_DIR", "").strip()
    if override:
        return Path(override).expanduser().resolve()
    state_home = os.environ.get("XDG_STATE_HOME", "").strip()
    base = Path(state_home).expanduser() if state_home else Path.home() / ".local" / "state"
    return (base / "rc-swarm" / repository_id()).resolve()


def validate_token(value: str, label: str) -> str:
    if not TOKEN.fullmatch(value):
        raise SystemExit(f"invalid {label} {value!r}")
    return value


def kind_code(value: str) -> str:
    code = ALIASES.get(value.strip().upper())
    if not code:
        raise SystemExit(f"unknown swarm kind {value!r}")
    return code


def protocol_source() -> Path:
    return Path(__file__).resolve().parent.parent / "docs" / "SWARM_PROTOCOL.md"


def initialize() -> Path:
    root = swarm_root()
    (root / "agents").mkdir(parents=True, exist_ok=True)
    (root / "threads").mkdir(parents=True, exist_ok=True)
    return root


def target_path(agent: str | None, thread: str | None) -> Path:
    root = initialize()
    if thread:
        return root / "threads" / f"{validate_token(thread, 'thread')}.s2"
    assert agent is not None
    return root / "agents" / f"{validate_token(agent, 'agent ID')}.s2"


def message(args: argparse.Namespace) -> str:
    return " ".join(args.message) if args.message else sys.stdin.read()


def append_line(path: Path, line: str) -> None:
    line = line.strip()
    if not line or "\n" in line or "\r" in line or "\x00" in line:
        raise SystemExit("SC2 entry must be one non-empty line")
    with path.open("a+", encoding="utf-8") as handle:
        fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
        handle.seek(0, os.SEEK_END)
        handle.write(line + "\n")
        handle.flush()
        os.fsync(handle.fileno())
        fcntl.flock(handle.fileno(), fcntl.LOCK_UN)


def compact_post(
    agent: str,
    kind: str,
    scope: str,
    refs: str,
    payload: str,
    thread: str | None,
) -> str:
    validate_token(agent, "agent ID")
    body = payload.strip()
    if not body:
        raise SystemExit("swarm payload must not be empty")
    prefix = []
    if scope.strip() not in {"", "-"}:
        prefix.append(f"sc:{scope.strip()}")
    if refs.strip() not in {"", "-"}:
        prefix.append(f"rf:{refs.strip()}")
    if prefix:
        body = ";".join(prefix + [body])
    char = KINDS[kind_code(kind)][0]
    return f"{agent} {char} {body}" if thread else f"{char} {body}"


def read(path: Path, tail: int | None) -> None:
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    if tail is not None:
        lines = text.splitlines()
        text = "\n".join(lines[-tail:]) + ("\n" if lines else "")
    sys.stdout.write(text)


def list_workspace() -> None:
    root = initialize()
    for label in ("agents", "threads"):
        values = sorted(path.stem for path in (root / label).glob("*.s2"))
        print(f"{label}: {','.join(values) if values else '-'}")


def prune(agent: str | None, thread: str | None) -> Path:
    path = target_path(agent, thread)
    path.unlink(missing_ok=True)
    return path


def decode_sc1(line: str) -> str:
    fields = [unquote(field) for field in line.strip().split("|", 6)]
    if len(fields) != 7:
        raise ValueError("invalid SC1 line")
    _, stamp, agent, kind, scope, refs, payload = fields
    label = KINDS[kind_code(kind)][1]
    return f"{agent} {label}\ntime:{stamp}\nscope:{scope}\nrefs:{refs}\n{payload}"


def decode_sc2(line: str, implicit_agent: str | None = None) -> str:
    line = line.strip()
    if line.startswith("@ "):
        return f"ALIASES {line[2:]}"
    first, sep, rest = line.partition(" ")
    if not sep:
        raise ValueError("invalid SC2 line")
    second, second_sep, remainder = rest.partition(" ")
    if second_sep and second in CHAR_KIND:
        agent = first
        kind = second
        body = remainder
    elif first in CHAR_KIND:
        agent = implicit_agent or "<implicit-agent>"
        kind = first
        body = rest
    else:
        raise ValueError("invalid SC2 line")
    _, label = CHAR_KIND[kind]
    return f"{agent} {label}\n{body}"


def decoded_text(line: str, implicit_agent: str | None = None) -> str:
    return decode_sc1(line) if line.startswith("SC1|") else decode_sc2(line, implicit_agent)


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(description=__doc__)
    sub = value.add_subparsers(dest="command", required=True)
    sub.add_parser("init")
    sub.add_parser("path")
    sub.add_parser("list")
    read_cmd = sub.add_parser("read")
    read_target = read_cmd.add_mutually_exclusive_group(required=True)
    read_target.add_argument("--agent")
    read_target.add_argument("--thread")
    read_target.add_argument("--protocol", action="store_true")
    read_cmd.add_argument("--tail", type=int)
    append_cmd = sub.add_parser("append")
    append_target = append_cmd.add_mutually_exclusive_group(required=True)
    append_target.add_argument("--agent")
    append_target.add_argument("--thread")
    append_cmd.add_argument("message", nargs="*")
    post = sub.add_parser("post")
    post.add_argument("--agent", required=True)
    post.add_argument("--kind", required=True)
    post.add_argument("--scope", default="-")
    post.add_argument("--refs", default="-")
    post.add_argument("--thread")
    post.add_argument("message", nargs="*")
    prune_cmd = sub.add_parser("prune")
    prune_target = prune_cmd.add_mutually_exclusive_group(required=True)
    prune_target.add_argument("--agent")
    prune_target.add_argument("--thread")
    decode = sub.add_parser("decode")
    decode.add_argument("--agent")
    decode.add_argument("line", nargs="*")
    return value


def main() -> None:
    args = parser().parse_args()
    if args.command == "init":
        print(initialize())
    elif args.command == "path":
        print(swarm_root())
    elif args.command == "list":
        list_workspace()
    elif args.command == "read":
        if args.protocol:
            path = protocol_source()
        else:
            path = target_path(args.agent, args.thread)
        read(path, args.tail)
    elif args.command == "append":
        path = target_path(args.agent, args.thread)
        append_line(path, message(args))
        print(path)
    elif args.command == "post":
        path = target_path(args.agent, args.thread)
        append_line(
            path,
            compact_post(args.agent, args.kind, args.scope, args.refs, message(args), args.thread),
        )
        print(path)
    elif args.command == "prune":
        print(prune(args.agent, args.thread))
    elif args.command == "decode":
        lines = args.line or [line for line in sys.stdin.read().splitlines() if line.strip()]
        for index, line in enumerate(lines):
            if index:
                print()
            print(decoded_text(line, args.agent))


if __name__ == "__main__":
    main()
