#!/usr/bin/env python3
"""Unit tests for the compact SC1 coordination protocol."""

from __future__ import annotations

import importlib.util
import pathlib
import unittest


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


if __name__ == "__main__":
    unittest.main()
