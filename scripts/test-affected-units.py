#!/usr/bin/env python3
"""Tests for affected-units.py."""

from __future__ import annotations

import importlib.util
import pathlib
import unittest


PATH = pathlib.Path(__file__).with_name("affected-units.py")
SPEC = importlib.util.spec_from_file_location("affected_units", PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AffectedUnitsTests(unittest.TestCase):
    def resolve(self, *paths: str) -> dict:
        return MODULE.resolve(list(paths))

    def test_runtime_change_builds_no_components(self) -> None:
        value = self.resolve("pkg/kernel/runtime.go")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], [])
        self.assertFalse(value["deps"])

    def test_command_change_rebuilds_image(self) -> None:
        value = self.resolve("cmd/rc-server/main.go")
        self.assertTrue(value["runtime"])
        self.assertTrue(value["image"])
        self.assertFalse(value["web"])

    def test_component_change_is_surgical(self) -> None:
        value = self.resolve("components/fixture-provider/export_wit_world/stub.go")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], ["fixture-provider"])
        self.assertEqual(value["profiles"], ["kernel-smoke"])

    def test_wit_change_rebuilds_consumers_and_runtime(self) -> None:
        value = self.resolve("wit/plugin.wit")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], sorted(MODULE.component_metadata()))

    def test_dependency_wit_change_rebuilds_only_consumers(self) -> None:
        value = self.resolve("wit/deps/diagnostics/diagnostics.wit")
        self.assertFalse(value["runtime"])
        self.assertEqual(
            value["components"],
            [
                "diagnostics-cli",
                "diagnostics-mesh",
                "diagnostics-reporter",
                "diagnostics-store",
                "diagnostics-ui",
                "execution-runtime",
                "scheduler",
            ],
        )

    def test_http_wit_change_rebuilds_runtime_and_web_consumer(self) -> None:
        value = self.resolve("wit/deps/http/http.wit")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], ["identity-http", "webui-app", "webui-shell"])

    def test_profile_change_compiles_nothing(self) -> None:
        value = self.resolve("profiles/core.toml")
        self.assertEqual(value["profiles"], ["core"])
        self.assertFalse(value["runtime"])
        self.assertEqual(value["components"], [])

    def test_web_change_does_not_build_runtime(self) -> None:
        value = self.resolve("web/client/pages.ts")
        self.assertTrue(value["web"])
        self.assertTrue(value["image"])
        self.assertFalse(value["runtime"])

    def test_component_tooling_rebuilds_components_only(self) -> None:
        value = self.resolve("scripts/build-component.sh")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], sorted(MODULE.component_metadata()))

    def test_module_change_marks_dependency_audit(self) -> None:
        value = self.resolve("go.mod")
        self.assertTrue(value["runtime"])
        self.assertTrue(value["deps"])

    def test_browser_script_does_not_build_runtime(self) -> None:
        value = self.resolve("scripts/windows-browser-e2e.ps1")
        self.assertTrue(value["browser"])
        self.assertFalse(value["runtime"])
        self.assertFalse(value["image"])

    def test_ci_workflow_change_runs_runtime_only(self) -> None:
        value = self.resolve(".github/workflows/ci.yml")
        self.assertTrue(value["runtime"])
        self.assertEqual(value["components"], [])

    def test_docs_change_is_docs_only(self) -> None:
        value = self.resolve("docs/DEVELOPMENT.md")
        self.assertTrue(value["docs"])
        self.assertFalse(value["runtime"])
        self.assertFalse(value["web"])


if __name__ == "__main__":
    unittest.main()
