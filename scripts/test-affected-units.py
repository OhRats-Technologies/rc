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

    def test_kernel_change_is_native_only(self) -> None:
        value = self.resolve("kernel/src/runtime.rs")
        self.assertTrue(value["kernel"])
        self.assertEqual(value["components"], [])
        self.assertFalse(value["legacy_rust"])

    def test_component_change_is_surgical(self) -> None:
        value = self.resolve("components/fixture-provider/src/lib.rs")
        self.assertFalse(value["kernel"])
        self.assertEqual(value["components"], ["fixture-provider"])
        self.assertEqual(value["profiles"], ["kernel-smoke"])

    def test_wit_change_rebuilds_consumers_and_kernel(self) -> None:
        value = self.resolve("wit/plugin.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            sorted(MODULE.component_metadata()),
        )

    def test_profile_change_compiles_nothing(self) -> None:
        value = self.resolve("profiles/canonical.toml")
        self.assertEqual(value["profiles"], ["kernel-smoke"])
        self.assertFalse(value["kernel"])
        self.assertEqual(value["components"], [])

    def test_web_change_does_not_build_kernel(self) -> None:
        value = self.resolve("web/client/pages.ts")
        self.assertTrue(value["web"])
        self.assertTrue(value["image"])
        self.assertFalse(value["kernel"])

    def test_component_tooling_rebuilds_components_only(self) -> None:
        value = self.resolve("scripts/build-component.sh")
        self.assertFalse(value["kernel"])
        self.assertEqual(value["components"], sorted(MODULE.component_metadata()))


if __name__ == "__main__":
    unittest.main()
