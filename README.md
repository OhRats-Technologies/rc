# [RC](https://rc.ohrats.party)

[![release](https://img.shields.io/github/v/release/OhRats-Technologies/rc?sort=semver&logo=github)](https://github.com/OhRats-Technologies/rc/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/OhRats-Technologies/rc/ci.yml?branch=main&label=CI&logo=github)](https://github.com/OhRats-Technologies/rc/actions/workflows/ci.yml)
[![license](https://img.shields.io/badge/license-AGPL--3.0-green.svg)](LICENSE)
[![website](https://img.shields.io/badge/website-rc.ohrats.party-purple.svg)](https://rc.ohrats.party)
[![support](https://img.shields.io/badge/sponsor-Open%20Collective-blue.svg)](https://opencollective.com/ohrats)

Secure remote control for macOS, Linux, and Windows through the browser, CLI,
API, and MCP. OpenSSH compatibility is advertised only where the target Node
supports it.

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

Licensed under the [GNU AGPL v3](LICENSE).
