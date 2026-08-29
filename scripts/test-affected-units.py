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

    def test_dependency_wit_change_rebuilds_only_consumers(self) -> None:
        value = self.resolve("wit/deps/diagnostics/diagnostics.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "diagnostics-cli",
                "diagnostics-mesh",
                "diagnostics-reporter",
                "diagnostics-store",
                "diagnostics-ui",
            ],
        )

    def test_http_wit_change_rebuilds_kernel_adapter_and_web_consumer(self) -> None:
        value = self.resolve("wit/deps/http/http.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(value["components"], ["identity-http", "webui-app", "webui-shell"])

    def test_storage_wit_change_rebuilds_kernel_adapter_and_fixture(self) -> None:
        value = self.resolve("wit/deps/storage/storage.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "api-credential-store",
                "authority-store",
                "device-store",
                "events-store",
                "identity-store",
                "ssh-policy-store",
                "storage-fixture",
                "workspace-store",
            ],
        )

    def test_process_wit_change_rebuilds_kernel_and_policy(self) -> None:
        value = self.resolve("wit/deps/process/process.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(value["components"], ["process-policy"])

    def test_mesh_wit_change_rebuilds_mesh_policy_units(self) -> None:
        value = self.resolve("wit/deps/mesh/mesh.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            ["mesh-policy", "mesh-policy-fixture"],
        )
        self.assertEqual(value["profiles"], ["mesh-smoke"])

    def test_crypto_wit_change_rebuilds_authority_crypto_units(self) -> None:
        value = self.resolve("wit/deps/crypto/crypto.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            ["authority-store", "crypto-ed25519", "key-custody-fixture"],
        )
        self.assertEqual(value["profiles"], ["authority-smoke", "keys-smoke"])

    def test_keys_wit_change_rebuilds_kernel_and_custody_units(self) -> None:
        value = self.resolve("wit/deps/keys/keys.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["key-custody-fixture", "key-custody-local"],
        )
        self.assertEqual(value["profiles"], ["keys-smoke"])

    def test_transport_wit_change_rebuilds_kernel_and_providers(self) -> None:
        value = self.resolve("wit/deps/transport/transport.wit")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["transport-test", "transport-webrtc"],
        )

    def test_identity_wit_change_rebuilds_identity_units(self) -> None:
        value = self.resolve("wit/deps/identity/identity.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "api-credential-fixture",
                "api-credential-store",
                "identity-fixture",
                "identity-http",
                "identity-store",
            ],
        )

    def test_session_wit_change_rebuilds_identity_units(self) -> None:
        value = self.resolve("wit/deps/session/session.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "api-credential-fixture",
                "identity-fixture",
                "identity-http",
                "identity-store",
                "webui-app",
            ],
        )

    def test_webauthn_wit_change_rebuilds_only_verifier_units(self) -> None:
        value = self.resolve("wit/deps/webauthn/webauthn.wit")
        self.assertFalse(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "api-credential-fixture",
                "identity-fixture",
                "identity-http",
                "identity-store",
                "webauthn-es256",
                "webauthn-fixture",
            ],
        )

    def test_profile_change_compiles_nothing(self) -> None:
        value = self.resolve("profiles/kernel-smoke.toml")
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

    def test_web_runtime_smoke_change_selects_its_units(self) -> None:
        value = self.resolve("scripts/smoke-web-runtime.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["diagnostics-store", "diagnostics-ui", "webui-shell"],
        )

    def test_storage_runtime_smoke_selects_only_its_fixture(self) -> None:
        value = self.resolve("scripts/smoke-storage.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(value["components"], ["storage-fixture"])

    def test_mesh_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-mesh-policy.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["mesh-policy", "mesh-policy-fixture"],
        )

    def test_authority_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-authority.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["authority-fixture", "authority-store", "crypto-ed25519"],
        )

    def test_keys_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-keys.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["crypto-ed25519", "key-custody-fixture", "key-custody-local"],
        )

    def test_artifact_cache_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-artifact-cache.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "artifact-cache-fixture-consumer",
                "artifact-cache-fixture-mesh-adapter",
                "artifact-cache-fixture-provider-v2",
                "artifact-cache-local",
                "artifact-cache-mesh",
            ],
        )

    def test_api_credential_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-api-credentials.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "api-credential-fixture",
                "api-credential-store",
                "identity-store",
                "webauthn-es256",
            ],
        )

    def test_authenticated_webui_runtime_smoke_selects_its_graph(self) -> None:
        value = self.resolve("scripts/smoke-authenticated-webui.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            [
                "identity-fixture",
                "identity-store",
                "webauthn-es256",
                "webui-app",
                "webui-shell",
            ],
        )

    def test_identity_runtime_smoke_selects_its_provider_and_fixture(self) -> None:
        value = self.resolve("scripts/smoke-identity.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["identity-fixture", "identity-store", "webauthn-es256"],
        )

    def test_webauthn_runtime_smoke_selects_its_provider_and_fixture(self) -> None:
        value = self.resolve("scripts/smoke-webauthn.sh")
        self.assertTrue(value["kernel"])
        self.assertEqual(
            value["components"],
            ["webauthn-es256", "webauthn-fixture"],
        )


if __name__ == "__main__":
    unittest.main()
