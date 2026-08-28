#!/usr/bin/env python3
"""Serve one component through plain HTTP and an authenticated OCI fixture."""

from __future__ import annotations

import hashlib
import http.server
import json
import pathlib
import sys


ARTIFACT = pathlib.Path(sys.argv[1]).read_bytes()
PORT_FILE = pathlib.Path(sys.argv[2])
ARTIFACT_DIGEST = "sha256:" + hashlib.sha256(ARTIFACT).hexdigest()
MANIFEST = json.dumps(
    {
        "schemaVersion": 2,
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "artifactType": "application/vnd.wasm.component.v1+wasm",
        "config": {
            "mediaType": "application/vnd.oci.empty.v1+json",
            "digest": "sha256:" + "0" * 64,
            "size": 2,
        },
        "layers": [
            {
                "mediaType": "application/vnd.wasm.component.v1+wasm",
                "digest": ARTIFACT_DIGEST,
                "size": len(ARTIFACT),
                "annotations": {
                    "org.opencontainers.image.title": "oci-demo.wasm"
                },
            }
        ],
    },
    separators=(",", ":"),
).encode()
MANIFEST_DIGEST = "sha256:" + hashlib.sha256(MANIFEST).hexdigest()


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/token?service=fixture&scope=repository%3Atest%2Fcomponent%3Apull":
            self.reply(200, b'{"token":"fixture-token"}', "application/json")
            return
        if self.path == "/http-demo.wasm":
            self.reply(200, ARTIFACT, "application/wasm")
            return
        if self.path.startswith("/v2/test/component/manifests/"):
            if self.headers.get("Authorization") != "Bearer fixture-token":
                realm = f'http://127.0.0.1:{self.server.server_port}/token'
                self.send_response(401)
                self.send_header(
                    "WWW-Authenticate",
                    f'Bearer realm="{realm}",service="fixture",scope="repository:test/component:pull"',
                )
                self.end_headers()
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/vnd.oci.image.manifest.v1+json")
            self.send_header("Docker-Content-Digest", MANIFEST_DIGEST)
            self.send_header("Content-Length", str(len(MANIFEST)))
            self.end_headers()
            self.wfile.write(MANIFEST)
            return
        if self.path == f"/v2/test/component/blobs/{ARTIFACT_DIGEST}":
            if self.headers.get("Authorization") != "Bearer fixture-token":
                self.send_error(401)
                return
            self.reply(200, ARTIFACT, "application/wasm")
            return
        self.send_error(404)

    def reply(self, status: int, body: bytes, content_type: str) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format: str, *_args: object) -> None:
        return


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
PORT_FILE.write_text(str(server.server_port), encoding="utf-8")
server.serve_forever()
