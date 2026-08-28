#!/usr/bin/env python3
"""Focused tests for SC2 coordination storage and direct authoring."""

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
    def test_direct_append_is_not_rewritten(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.dict(os.environ, {"RC_SWARM_DIR": directory}):
                path = SWARM.target_path(None, "upd")
                line = "u s hard+; tst:rust/foc run; >smoke,push"
                SWARM.append_line(path, line)
                self.assertEqual(path.read_text(), line + "\n")

    def test_transition_post_emits_sc2_without_timestamp_or_thread(self) -> None:
        line = SWARM.compact_post(
            "sol-updater-4c91",
            "STA",
            "K/platform",
            "@web-migrate-a73f",
            "hard+; >smoke",
            "updater-platform",
        )
        self.assertEqual(
            line,
            "sol-updater-4c91 s sc:K/platform;rf:@web-migrate-a73f;hard+; >smoke",
        )

    def test_agent_record_omits_agent_identity(self) -> None:
        self.assertEqual(
            SWARM.compact_post("sol-updater-4c91", "TST", "-", "-", "ok:kern", None),
            "t ok:kern",
        )

    def test_optional_decoder_understands_contextual_lines(self) -> None:
        self.assertIn("u STATUS", SWARM.decoded_text("u s hard+; >smoke"))
        self.assertIn(
            "sol-updater-4c91 STATUS",
            SWARM.decoded_text("s hard+; >smoke", "sol-updater-4c91"),
        )
        self.assertEqual(SWARM.decoded_text("@ u=sol-updater-4c91"), "ALIASES u=sol-updater-4c91")

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

    def test_list_and_prune_use_sc2_files_only(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.dict(os.environ, {"RC_SWARM_DIR": directory}):
                root = SWARM.initialize()
                SWARM.append_line(root / "threads" / "live.s2", "u s run")
                (root / "threads" / "legacy.md").write_text("old\n")
                path = SWARM.prune(None, "live")
            self.assertFalse(path.exists())
            self.assertTrue((root / "threads" / "legacy.md").exists())


if __name__ == "__main__":
    unittest.main()
