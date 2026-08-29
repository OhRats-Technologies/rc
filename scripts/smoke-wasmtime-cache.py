#!/usr/bin/env python3
"""Verify the kernel's persistent Wasmtime compilation cache."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parent.parent
KERNEL = ROOT / "kernel/target/debug/rc-kernel"
FIXTURES = ROOT / "dist/components"


def run(components: Path, cache: Path) -> tuple[float, str]:
    env = os.environ.copy()
    env["RC_WASMTIME_CACHE_DIR"] = str(cache)
    started = time.monotonic()
    result = subprocess.run(
        [str(KERNEL), "--component-dir", str(components), "components"],
        cwd=ROOT,
        env=env,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return time.monotonic() - started, result.stdout


def cache_files(cache: Path) -> set[Path]:
    return {
        path.relative_to(cache)
        for path in cache.rglob("*")
        if path.is_file() and path.stat().st_size > 0
    }


def main() -> None:
    required = [
        "fixture-provider.wasm",
        "fixture-consumer.wasm",
        "call-context-consumer.wasm",
        "fixture-broken.wasm",
    ]
    missing = [name for name in required if not (FIXTURES / name).is_file()]
    if missing:
        raise SystemExit(f"missing fixture artifacts: {', '.join(missing)}")
    if not KERNEL.is_file():
        raise SystemExit(f"missing kernel binary: {KERNEL}")

    with tempfile.TemporaryDirectory(prefix="rc-wasmtime-cache-") as temporary:
        root = Path(temporary)
        components = root / "components"
        cache = root / "compile-cache"
        components.mkdir()
        for source, destination in (
            ("fixture-provider.wasm", "provider.wasm"),
            ("fixture-consumer.wasm", "consumer.wasm"),
            ("call-context-consumer.wasm", "caller.wasm"),
        ):
            shutil.copyfile(FIXTURES / source, components / destination)

        cold, cold_output = run(components, cache)
        cold_files = cache_files(cache)
        assert cold_files, "cold invocation did not populate the Wasmtime cache"
        warm, warm_output = run(components, cache)
        warm_files = cache_files(cache)
        assert warm_output == cold_output
        assert warm_files == cold_files, "warm invocation unexpectedly changed cache keys"
        assert warm <= cold * 0.90, f"warm cache was not material: cold={cold:.3f}s warm={warm:.3f}s"

        shutil.copyfile(FIXTURES / "fixture-broken.wasm", components / "provider.wasm")
        changed, changed_output = run(components, cache)
        changed_files = cache_files(cache)
        assert changed_output != cold_output, "changed component bytes were not observed"
        assert changed_files > warm_files, "changed component bytes did not add a cache entry"
        changed_warm, changed_warm_output = run(components, cache)
        assert changed_warm_output == changed_output
        assert cache_files(cache) == changed_files
        assert changed_warm <= changed * 0.90, (
            "changed component did not warm: "
            f"cold={changed:.3f}s warm={changed_warm:.3f}s"
        )

        total = sum((cache / path).stat().st_size for path in changed_files)
        print(
            "wasmtime cache smoke: ok; "
            f"initial={cold:.3f}s/{warm:.3f}s "
            f"changed={changed:.3f}s/{changed_warm:.3f}s "
            f"files={len(changed_files)} bytes={total}"
        )


if __name__ == "__main__":
    main()
