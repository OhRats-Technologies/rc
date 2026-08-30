# [RC](https://rc.ohrats.party)

Secure remote control for macOS and Linux through the browser, CLI, API, MCP, and OpenSSH.

RC is about halfway through a migration to a small native Wasmtime kernel and independently built WebAssembly components. Components can be written in any language that targets the WebAssembly Component Model, and component updates do not require the RC kernel to restart.

## Install

```sh
curl -fsSL https://rc.ohrats.party/install.sh | sh
```

For a Node, generate an enrollment command from **Devices → Enroll device**.

```sh
rc login --url https://rc.ohrats.party
rc devices
rc shell DEVICE
rc run DEVICE -- uname -a
rc update
rc upgrade
```

`rc update` changes managed components. `rc upgrade` changes the native platform and core component bundle.

See [`ROADMAP.md`](ROADMAP.md) for the component migration and [`docs/`](docs/) for architecture, security, operations, and API details.

Licensed under the [GNU AGPL v3](LICENSE).
