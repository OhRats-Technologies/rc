# [RC](https://rc.ohrats.party)

[![release](https://img.shields.io/github/v/release/OhRats-Technologies/rc?sort=semver&logo=github)](https://github.com/OhRats-Technologies/rc/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/OhRats-Technologies/rc/ci.yml?branch=main&label=CI&logo=github)](https://github.com/OhRats-Technologies/rc/actions/workflows/ci.yml)
[![license](https://img.shields.io/badge/license-AGPL--3.0-green.svg)](LICENSE)
[![website](https://img.shields.io/badge/website-rc.ohrats.party-purple.svg)](https://rc.ohrats.party)
[![support](https://img.shields.io/badge/sponsor-Open%20Collective-blue.svg)](https://opencollective.com/ohrats)

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
