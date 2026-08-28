#!/usr/bin/env python3
"""Unit tests for the compact SC1 coordination protocol."""

from __future__ import annotations

import importlib.util
import os
import pathlib
import tempfile
import unittest
from unittest import mock


PATH = pathlib.Path(__file__).with_name("swarm.py")
SPEC = importlib.util.spec_from_file_location("swarm", PATH)
assert SPEC and SPEC.loader
SWARM = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SWARM)


class SwarmProtocolTests(unittest.TestCase):
    def test_round_trip_preserves_delimiters_and_lines(self) -> None:
        line = SWARM.encode(
            "node-runtime-8d31",
            "claim",
            "kernel/src/node|wit",
            "@web-migrate-a73f",
            "own=transport%policy\nnext=tests",
            "20260828T230000Z",
        )
        fields = SWARM.parse(line)
        self.assertEqual(fields[1], "20260828T230000Z")
        self.assertEqual(fields[2], "node-runtime-8d31")
        self.assertEqual(fields[3], "CLM")
        self.assertEqual(fields[4], "kernel/src/node|wit")
        self.assertEqual(fields[6], "own=transport%policy\nnext=tests")

    def test_kind_aliases_are_canonical(self) -> None:
        self.assertEqual(SWARM.kind_code("task"), "CLM")
        self.assertEqual(SWARM.kind_code("handoff"), "HOF")
        self.assertEqual(SWARM.kind_code("CFT"), "CFT")

    def test_decode_is_human_readable(self) -> None:
        line = SWARM.encode(
            "node-runtime-8d31",
            "ACK",
            "-",
            "#coord-v2",
            "protocol=SC1",
            "20260828T230000Z",
        )
        text = SWARM.decoded_text(line)
        self.assertIn("node-runtime-8d31 ACK", text)
        self.assertIn("message: protocol=SC1", text)

    def test_explicit_state_directory_wins(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.dict(os.environ, {"RC_SWARM_DIR": directory}):
                self.assertEqual(SWARM.swarm_root(), pathlib.Path(directory).resolve())

    def test_default_state_is_outside_git_metadata(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            common = pathlib.Path(directory) / "rc" / ".git"
            state = pathlib.Path(directory) / "state"
            with mock.patch.object(SWARM, "common_dir", return_value=common):
                with mock.patch.dict(
                    os.environ,
                    {"XDG_STATE_HOME": str(state), "RC_SWARM_DIR": ""},
                ):
                    root = SWARM.swarm_root()
            self.assertEqual(root.parent, (state / "rc-swarm").resolve())
            self.assertNotIn(".git", root.parts)

    def test_initialize_creates_only_live_state_directories(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.dict(os.environ, {"RC_SWARM_DIR": directory}):
                root = SWARM.initialize()
            self.assertTrue((root / "agents").is_dir())
            self.assertTrue((root / "threads").is_dir())
            self.assertEqual(
                sorted(path.name for path in root.iterdir()),
                ["agents", "threads"],
            )


if __name__ == "__main__":
    unittest.main()
