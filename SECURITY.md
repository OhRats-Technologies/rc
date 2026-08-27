# Security

## Reporting a vulnerability

Do not publish exploit details, credentials, private keys, terminal output, or customer data in a public issue. Use GitHub private vulnerability reporting when available, or contact the repository owners through an established private channel. Include the affected version, surface, prerequisites, impact, and a minimal reproduction that uses disposable credentials and machines.

Preserve relevant logs and artifacts, but redact setup tokens, enrollment tokens, browser cookies, API secrets, CLI/Node state, OAuth codes, TURN credentials, command content, and process output.

## Supported versions

During the v0.16 alpha, only the newest published v0.16 prerelease is supported. Security fixes move forward to a new immutable tag; published tags are not rewritten. v0.15 is a separate legacy deployment with a separate database and agent.

## Security model

- Human browser/CLI terminal control uses authenticated key agreement and application-layer AES-GCM over WebRTC DataChannels. There is no hosted terminal fallback.
- The Node validates control grants, device identity, process permits, and RC Lock locally before execution.
- Browser accounts use passkeys. Credential creation, revocation, and destructive actions require fresh passkey step-up.
- CLI and API clients use Ed25519 proof-of-possession signatures bound to method, path/query, timestamp, nonce, and body digest.
- Node HTTP uses a separate Ed25519 signature domain and replay protection.
- Enrollment tokens, invites, OAuth codes, nonces, and step-up tokens are one-time or replay-resistant and expire.
- Workspace roles and client scopes are both enforced. Server-side process allocation alone is not execution authority.
- Device deletion records a revocation tombstone.
- Public signup is off by default and requires both Turnstile keys when enabled.
- Security headers include CSP, frame denial, MIME sniffing denial, no-referrer, restrictive permissions policy, and HSTS on HTTPS origins.
- Private HTML, JSON, and SSE responses default to `Cache-Control: no-store`.
- Request bodies and hosted MCP output are bounded.

SSH and MCP are explicit hosted-relay surfaces. SSH is authenticated by a registered public key bound to a control client. MCP uses OAuth 2.0 authorization code flow with PKCE, explicit machine/tool scopes, and passkey-backed grants. Treat these paths as different trust boundaries from direct browser/CLI control.

## Secrets and local state

The server data directory and database use owner-only Unix permissions. `~/.config/rc/device.json`, `account.json`, and `lock.json` contain high-value secrets and are also owner-only. Never sync them through consumer cloud storage, paste them into support channels, or include them in diagnostics.

API secrets are shown once. The `rcsk_...` value contains a private key; rotate it immediately if copied to an untrusted machine or log. A server backup contains passkey and authorization records but not Node private identity keys.

## Deployment requirements

- Use HTTPS for every non-localhost deployment and keep `PUBLIC_URL` stable.
- Generate a high-entropy `RC_SETUP_TOKEN`; open the exact setup link and avoid retaining it in shell history.
- Set `RC_TRUST_PROXY=1` only when the proxy removes untrusted forwarding headers.
- Restrict access to the SQLite volume, TURN API token, and container environment.
- Keep host time synchronized because signed requests have a 60-second acceptance window.
- Back up before upgrades and test restores in isolation.
- Monitor failed authentication, replay conflicts, rate limits, Node churn, and unexpected service restarts.
- Revoke credentials and device records before forensic re-enrollment after compromise.

## Cryptographic changes

Changes to domains, canonical request payloads, grants, key derivation, nonces, counters, or serialization require deterministic fixtures, positive/negative cross-component tests, replay tests, and compatibility documentation. Never reuse a key/nonce pair or silently downgrade to plaintext or a hosted fallback.

CI audits the locked Rust graph against RustSec and the Bun graph against the registry advisory service. New vulnerabilities, yanked crates, or informational warnings fail validation. The sole documented exception is the unmaintained-only `bincode 1.3.3` notice inherited from `webrtc` through `dtls`; `.cargo/audit.toml` records the exact advisory and upstream path.
