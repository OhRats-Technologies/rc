#!/usr/bin/env python3
"""Read and append compact SC1 messages in RC's shared swarm workspace."""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import unquote


TOKEN = re.compile(r"^[a-z0-9][a-z0-9._-]{1,63}$")
KINDS = {
    "CLM": "CLAIM",
    "ACK": "ACK",
    "PRP": "PROPOSAL",
    "QRY": "QUESTION",
    "RSP": "RESPONSE",
    "STA": "STATUS",
    "BLK": "BLOCKED",
    "CFT": "CONFLICT",
    "WRN": "WARNING",
    "TST": "TEST",
    "PUB": "PUBLISHED",
    "HOF": "HANDOFF",
    "RES": "RESOLVED",
    "CAN": "CANCELLED",
}
ALIASES = {value: key for key, value in KINDS.items()} | {
    key: key for key in KINDS
} | {"TASK": "CLM", "CLAIM": "CLM", "HANDOFF": "HOF", "BLOCKER": "BLK"}


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
        raise SystemExit(f"unknown SC1 kind {value!r}")
    return code


def escape(value: str) -> str:
    return (
        value.replace("%", "%25")
        .replace("|", "%7C")
        .replace("\r", "%0D")
        .replace("\n", "%0A")
    )


def encode(
    agent: str,
    kind: str,
    scope: str,
    refs: str,
    payload: str,
    timestamp: str | None = None,
) -> str:
    validate_token(agent, "agent ID")
    payload = payload.strip()
    if not payload:
        raise SystemExit("SC1 payload must not be empty")
    timestamp = timestamp or datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    fields = [
        "SC1",
        timestamp,
        agent,
        kind_code(kind),
        scope.strip() or "-",
        refs.strip() or "-",
        payload,
    ]
    return "|".join(escape(field) for field in fields)


def parse(line: str) -> list[str]:
    fields = [unquote(field) for field in line.strip().split("|", 6)]
    if len(fields) != 7 or fields[0] != "SC1":
        raise ValueError("not a valid SC1 line")
    validate_token(fields[2], "agent ID")
    fields[3] = kind_code(fields[3])
    return fields


def decoded_text(line: str) -> str:
    _, timestamp, agent, kind, scope, refs, payload = parse(line)
    stamp = datetime.strptime(timestamp, "%Y%m%dT%H%M%SZ").replace(
        tzinfo=timezone.utc
    )
    return "\n".join(
        [
            f"[{stamp.isoformat().replace('+00:00', 'Z')}] {agent} {KINDS[kind]}",
            f"scope: {scope}",
            f"refs: {refs}",
            f"message: {payload}",
        ]
    )


def protocol_source() -> Path:
    return Path(__file__).resolve().parent.parent / "docs" / "SWARM_PROTOCOL.md"


def initialize(refresh: bool = False) -> Path:
    root = swarm_root()
    (root / "agents").mkdir(parents=True, exist_ok=True)
    (root / "threads").mkdir(parents=True, exist_ok=True)
    _ = refresh
    return root


def target_path(agent: str, thread: str | None) -> Path:
    root = initialize()
    if thread:
        return root / "threads" / f"{validate_token(thread, 'thread')}.md"
    return root / "agents" / f"{validate_token(agent, 'agent ID')}.md"


def append(path: Path, line: str, heading: str) -> None:
    with path.open("a+", encoding="utf-8") as handle:
        fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
        handle.seek(0, os.SEEK_END)
        if handle.tell() == 0:
            handle.write(f"# {heading}\n\n")
        handle.write(line + "\n")
        handle.flush()
        os.fsync(handle.fileno())
        fcntl.flock(handle.fileno(), fcntl.LOCK_UN)


def read(path: Path, tail: int | None) -> None:
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    if tail is not None:
        lines = text.splitlines()
        text = "\n".join(lines[-tail:]) + ("\n" if lines else "")
    sys.stdout.write(text)


def list_workspace() -> None:
    root = initialize()
    for label in ("agents", "threads"):
        values = sorted(path.stem for path in (root / label).glob("*.md"))
        print(f"{label}: {','.join(values) if values else '-'}")


def prune(agent: str | None, thread: str | None) -> Path:
    root = initialize()
    if thread:
        path = root / "threads" / f"{validate_token(thread, 'thread')}.md"
    else:
        assert agent is not None
        path = root / "agents" / f"{validate_token(agent, 'agent ID')}.md"
    path.unlink(missing_ok=True)
    return path


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(description=__doc__)
    sub = value.add_subparsers(dest="command", required=True)
    init = sub.add_parser("init")
    init.add_argument("--refresh-protocol", action="store_true")
    sub.add_parser("path")
    sub.add_parser("list")
    read_cmd = sub.add_parser("read")
    target = read_cmd.add_mutually_exclusive_group(required=True)
    target.add_argument("--agent")
    target.add_argument("--thread")
    target.add_argument("--protocol", action="store_true")
    read_cmd.add_argument("--tail", type=int)
    prune_cmd = sub.add_parser("prune")
    prune_target = prune_cmd.add_mutually_exclusive_group(required=True)
    prune_target.add_argument("--agent")
    prune_target.add_argument("--thread")
    for name in ("post", "encode"):
        command = sub.add_parser(name)
        command.add_argument("--agent", required=True)
        command.add_argument("--kind", required=True)
        command.add_argument("--scope", default="-")
        command.add_argument("--refs", default="-")
        if name == "post":
            command.add_argument("--thread")
        command.add_argument("message", nargs="*")
    decode = sub.add_parser("decode")
    decode.add_argument("line", nargs="*")
    return value


def message(args: argparse.Namespace) -> str:
    return " ".join(args.message) if args.message else sys.stdin.read()


def main() -> None:
    args = parser().parse_args()
    if args.command == "init":
        print(initialize(args.refresh_protocol))
    elif args.command == "path":
        print(swarm_root())
    elif args.command == "list":
        list_workspace()
    elif args.command == "read":
        root = initialize()
        if args.protocol:
            path = protocol_source()
        elif args.thread:
            path = root / "threads" / f"{validate_token(args.thread, 'thread')}.md"
        else:
            path = root / "agents" / f"{validate_token(args.agent, 'agent ID')}.md"
        read(path, args.tail)
    elif args.command == "prune":
        print(prune(args.agent, args.thread))
    elif args.command in {"post", "encode"}:
        line = encode(args.agent, args.kind, args.scope, args.refs, message(args))
        if args.command == "encode":
            print(line)
        else:
            path = target_path(args.agent, args.thread)
            append(path, line, f"thread:{args.thread}" if args.thread else f"agent:{args.agent}")
            print(path)
    elif args.command == "decode":
        lines = args.line or [line for line in sys.stdin.read().splitlines() if line.strip()]
        for index, line in enumerate(lines):
            if index:
                print()
            print(decoded_text(line))


if __name__ == "__main__":
    main()
