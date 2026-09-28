#!/usr/bin/env python3
"""Compile native code concurrently; wait for same-run Wasm only when needed."""
import json
import os
import subprocess
import time

EXPECTED = {
    f"rc-native-runtime-{name}" for name in (
        "diagnostics-store", "diagnostics-cli", "process-policy", "shell",
        "execution-runtime", "scheduler", "transport-webrtc",
    )
}


def main():
    route = f"repos/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{os.environ['GITHUB_RUN_ID']}"

    def fetch(path):
        return json.loads(subprocess.check_output(["gh", "api", route + path], text=True))

    deadline = time.monotonic() + 900
    while time.monotonic() < deadline:
        artifacts = fetch("/artifacts?per_page=100")["artifacts"]
        missing = EXPECTED - {item["name"] for item in artifacts if not item["expired"]}
        if not missing:
            print("All portable runtime components are available from this workflow run.")
            return
        jobs = fetch("/jobs?per_page=100")["jobs"]
        failed = [job["name"] for job in jobs
                  if job["name"].startswith("native-runtime-components /")
                  and job["conclusion"] not in (None, "success")]
        if failed:
            raise SystemExit(f"Portable component build failed: {failed}")
        print(f"Waiting for {', '.join(sorted(missing))}", flush=True)
        time.sleep(10)
    raise SystemExit("Timed out waiting for portable components from this workflow run")


if __name__ == "__main__":
    main()
