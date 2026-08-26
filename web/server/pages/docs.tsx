import type { ReactNode } from "react";
import { PUBLIC_URL } from "../../../src/config";
import { htmlDocument } from "../document";
import { Arrow, PublicFooter, PublicNav } from "../public";

function CopyField({ value, prefix = "$" }: { value: string; prefix?: string }) {
  return <div className="or-copy-field" data-copy-value={value} title={value}>
    {prefix && <span className="or-copy-prefix">{prefix}</span>}<code>{value}</code>
    <button className="or-copy-button copy-value" type="button" aria-label="Copy"><span className="or-copy-icon" aria-hidden="true"/></button>
  </div>;
}

function CodeBlock({ children }: { children: string }) {
  return <pre className="public-code"><code>{children}</code></pre>;
}

function page(active: string, title: string, intro: string, children: ReactNode, copy = false) {
  return htmlDocument({
    title, description: intro, canonicalPath: active === "docs" ? "/docs" : `/docs/${active}`,
    styles: ["public"], scripts: copy ? ["copy"] : [], indexable: true, publicSite: true,
    body: <div className="public-site"><PublicNav active={active}/><main className="container listing-page public-doc">
      <header className="public-doc-header"><h1>{title}</h1><p className="listing-copy">{intro}</p></header>
      <div className="public-doc-body">{children}</div>
    </main><PublicFooter/></div>,
  });
}

function DocSection({ title, children }: { title: string; children: ReactNode }) {
  return <section className="public-doc-section"><h2>{title}</h2><div className="public-doc-copy">{children}</div></section>;
}

export function docsPage() {
  return page("docs", "Docs", "Account setup, device enrollment, encryption, authorization, and data handling.", <>
    <DocSection title="Create an account and enroll a machine">
      <p>Accounts require a workspace invitation. Open the invite, choose a user name, and create a passkey.</p>
      <p>To enroll a machine, open <strong>Devices</strong>, choose <strong>Enroll device</strong>, create an enrollment command, and run that command on macOS or Linux. Enrollment tokens are one-time credentials.</p>
      <p>The installer places <code>ohrats-rc</code> in <code>~/.local/bin</code>. After enrollment it installs a per-user background service: a LaunchAgent on macOS or a user systemd unit on Linux.</p>
      <div className="public-doc-actions"><a className="header-link" href="/login">Sign in <Arrow/></a><a className="header-link" href="/docs/cli">CLI reference <Arrow/></a></div>
    </DocSection>

    <DocSection title="Browser and CLI encryption">
      <p>Browser and CLI control sessions are encrypted between the client and RC Node. The RC server relays ciphertext.</p>
      <ol>
        <li>The client creates a fresh X25519 key pair for the session.</li>
        <li>The Node has a pinned static X25519 transport key and creates a fresh X25519 key pair for the session.</li>
        <li>The client and Node derive two X25519 shared secrets: client↔Node-static and client↔Node-ephemeral.</li>
        <li>The two secrets are combined and passed through HKDF-SHA256. The derivation is bound to the session challenge, device ID, and client ID.</li>
        <li>The resulting AES-256-GCM key encrypts commands, stdin, resize/signal messages, stdout/stderr, and process lifecycle messages.</li>
      </ol>
      <p>The Node signs the handshake with its Ed25519 device identity. The signed data binds the challenge, device, client, client ephemeral key, Node static transport key, Node ephemeral key, and session ID. The client rejects a changed pinned identity or transport key.</p>
      <p>Encrypted frames use direction-specific nonces and authenticated additional data containing the session ID, sequence number, and direction. Sequence numbers must increase by exactly one; gaps or replays close the session.</p>
      <p>A later compromise of the Node's long-term X25519 transport key alone is not sufficient to decrypt a recorded session because the fresh Node ephemeral private key is not retained.</p>
    </DocSection>

    <DocSection title="RC Lock">
      <p>Each Node stores an RC Lock authority snapshot containing workspace members and roles, passkey public credentials, API signing keys with scopes and expiry, and hashes of active execution-capable MCP grants.</p>
      <p>The first lock is a trust-on-first-use bootstrap from the RC instance used for enrollment. After that, a lock update must reference the Node's current generation and lock hash and be signed by a passkey-backed control identity belonging to a user who is already an Owner in the current snapshot.</p>
      <p>The Node rejects stale transitions, invalid signatures, expired API/MCP credentials, and execution credentials that are absent from its local snapshot. Changing the hosted database alone does not add execution authority to a locked Node.</p>
      <p>When a new lock generation is accepted, existing encrypted control sessions are invalidated and must reconnect against the new authority.</p>
    </DocSection>

    <DocSection title="What RC stores">
      <p>SQLite stores account, workspace, device, authority, process, Action, audit, and authorization metadata.</p>
      <p>New browser and CLI processes do not persist command, working directory, stdin, stdout, or stderr in process history. The server sees encrypted control frames only.</p>
      <p>MCP Terminal is different because standard remote MCP does not use RC's custom end-to-end transport. MCP command/output data passes through bounded server memory while the call is active, but it is not written to SQLite process history.</p>
    </DocSection>

    <DocSection title="Authorization lifetimes">
      <table className="public-doc-table"><thead><tr><th>Authorization</th><th>Default</th></tr></thead><tbody>
        <tr><td>Web session</td><td>30 days</td></tr>
        <tr><td>CLI session</td><td>Until revoked</td></tr>
        <tr><td>MCP grant</td><td>Until revoked</td></tr>
        <tr><td>API key</td><td>Until revoked</td></tr>
        <tr><td>MCP access token</td><td>15 minutes</td></tr>
      </tbody></table>
      <p>Finite issuance presets are 1 hour, 1 day, 7 days, 30 days, 90 days, 180 days, and 1 year. CLI, MCP, and API also support until revoked. Protocol challenges, authorization codes, and step-up tokens remain short-lived.</p>
    </DocSection>
  </>);
}

export function mcpDocsPage() {
  const endpoint = `${PUBLIC_URL}/mcp`;
  const codex = `codex mcp add ohrats-rc --url ${endpoint} --oauth-resource ${endpoint}`;
  const cursor = `{
  "mcpServers": {
    "ohrats-rc": { "url": "${endpoint}" }
  }
}`;
  const vscode = `{
  "servers": {
    "ohrats-rc": { "type": "http", "url": "${endpoint}" }
  }
}`;
  const agy = `{
  "mcpServers": {
    "ohrats-rc": { "serverUrl": "${endpoint}" }
  }
}`;
  return page("mcp", "MCP", "Endpoint, client configuration, scopes, tools, OAuth, and process behavior.", <>
    <DocSection title="Endpoint">
      <CopyField value={endpoint} prefix=""/>
      <p>All users connect to the same endpoint and authenticate with OAuth. The server name is <strong>OhRats RC</strong>; use <code>ohrats-rc</code> when a client requires an identifier without spaces.</p>
    </DocSection>

    <DocSection title="Codex">
      <CopyField value={codex}/>
      <CopyField value="codex mcp login --scopes mcp:observe,mcp:actions,mcp:terminal ohrats-rc"/>
      <p>Request only the scopes you need. For example, omit <code>mcp:terminal</code> if the agent only needs saved Actions.</p>
      <p>RC has been tested end to end with Codex 0.147. If that version lists the server under <code>codex mcp list</code> but does not expose its tools to an agent, enable <code>mcp_2026_07_28</code> in Codex before retrying.</p>
      <CodeBlock>{`[features]
mcp_2026_07_28 = true`}</CodeBlock>
    </DocSection>

    <DocSection title="Claude Code">
      <CopyField value={`claude mcp add --scope user --transport http ohrats-rc ${endpoint}`}/>
      <CopyField value="claude mcp login ohrats-rc"/>
      <p>You can also use <code>/mcp</code> inside Claude Code to inspect the server and authentication state.</p>
    </DocSection>

    <DocSection title="Cursor">
      <p>Add the server to <code>~/.cursor/mcp.json</code> for all projects or <code>.cursor/mcp.json</code> for one project.</p>
      <CodeBlock>{cursor}</CodeBlock>
      <CopyField value="cursor-agent mcp login ohrats-rc"/>
    </DocSection>

    <DocSection title="Visual Studio Code">
      <p>Add the server to the user MCP configuration or to <code>.vscode/mcp.json</code> in a workspace.</p>
      <CodeBlock>{vscode}</CodeBlock>
      <p>Start the server from <strong>MCP: List Servers</strong> or use it from Agent mode. VS Code handles remote MCP authentication in the browser.</p>
    </DocSection>

    <DocSection title="Antigravity CLI (agy)">
      <p>Add the server to <code>~/.gemini/config/mcp_config.json</code>, or use <code>.agents/mcp_config.json</code> for one workspace.</p>
      <CodeBlock>{agy}</CodeBlock>
      <p>Start <code>agy</code> and open <code>/mcp</code> to inspect configured servers and authenticate when required.</p>
    </DocSection>

    <DocSection title="Client compatibility">
      <p>Codex has passed RC's OAuth and remote-execution acceptance test. The other examples use the current configuration format for each client but have not yet passed an RC execution acceptance test.</p>
    </DocSection>

    <DocSection title="Scopes and tools">
      <table className="public-doc-table"><thead><tr><th>Scope</th><th>Tools</th><th>Meaning</th></tr></thead><tbody>
        <tr><td><code>mcp:observe</code></td><td><code>machines_list</code>, <code>process_status</code></td><td>Read granted machine metadata and status/output for MCP processes created by the same grant.</td></tr>
        <tr><td><code>mcp:actions</code></td><td><code>actions_list</code>, <code>action_run</code></td><td>List and run saved Actions captured when the grant was approved.</td></tr>
        <tr><td><code>mcp:terminal</code></td><td><code>process_run</code></td><td>Run an arbitrary shell command on an explicitly granted machine.</td></tr>
      </tbody></table>
      <p>Terminal is not selected by default. Actions are bound to the Action ID plus a hash of its command and working directory; editing an Action requires re-authorization before MCP can run it again.</p>
    </DocSection>

    <DocSection title="OAuth and Node verification">
      <p>RC publishes Protected Resource Metadata and authorization-server metadata. Clients use PKCE S256 and resource-bound access tokens. Access tokens are short-lived; refresh tokens rotate and cannot outlive the MCP grant.</p>
      <p>During approval, RC asks which machines, scopes, and grant lifetime to authorize. Execution-capable grants require an Owner and a fresh passkey ceremony. The browser control identity signs the exact MCP grant, and its hash is synchronized into RC Lock.</p>
      <p>Before execution, the Node verifies that the grant signature is valid, the signer is a current Owner, the current device is selected, the required scope is present, the grant is active in RC Lock, and any saved Action hash matches.</p>
    </DocSection>

    <DocSection title="Terminal output">
      <p><code>process_run</code> waits up to the requested timeout and returns stdout/stderr plus process status. If the process is still running, use <code>process_status</code> with the returned process ID and <code>nextOffset</code> to read later output without repeating earlier bytes.</p>
      <p>MCP process output is bounded to 256 KiB in server memory. Completed buffers expire after five minutes; inactive running buffers expire after thirty minutes. The buffer is not persisted to SQLite.</p>
    </DocSection>
  </>, true);
}

export function apiDocsPage() {
  const canonical = `rc-api-v1
<key-id>
<unix-timestamp-seconds>
<nonce>
<HTTP-METHOD>
<path-and-query>
<hex-sha256-body>`;
  return page("api", "API", "HTTP API authentication, scopes, request signing, and reference.", <>
    <DocSection title="Create an API key">
      <p>Open <a href="/api">API keys</a>, create a key, choose scopes and lifetime, and complete passkey step-up. The browser generates an Ed25519 key pair and sends RC only the public key.</p>
      <p>The private key is shown once as <code>rcsk_&lt;key-id&gt;_&lt;pkcs8-private-key-base64url&gt;</code>. Store it as a secret. RC cannot recover it later.</p>
    </DocSection>

    <DocSection title="Scopes">
      <table className="public-doc-table"><thead><tr><th>Scope</th><th>Allows</th></tr></thead><tbody>
        <tr><td><code>read</code></td><td>GET requests to account/workspace/device/action/process resources.</td></tr>
        <tr><td><code>execute</code></td><td>Allocate device processes and run saved Actions.</td></tr>
        <tr><td><code>manage-devices</code></td><td>Create enrollments and modify/remove devices.</td></tr>
        <tr><td><code>manage-workspaces</code></td><td>Create/modify workspaces and Actions.</td></tr>
      </tbody></table>
      <p>Passkey and API-key administration require a human browser session and cannot be performed with an API key.</p>
    </DocSection>

    <DocSection title="Sign a request">
      <p>Every API-key request includes four headers:</p>
      <table className="public-doc-table"><tbody>
        <tr><th><code>X-RC-Key-ID</code></th><td>The key ID from the <code>rcsk_</code> credential.</td></tr>
        <tr><th><code>X-RC-Timestamp</code></th><td>Current Unix time in seconds. RC accepts a 60-second clock window.</td></tr>
        <tr><th><code>X-RC-Nonce</code></th><td>A new random value for every request. Reuse is rejected.</td></tr>
        <tr><th><code>X-RC-Signature</code></th><td>Base64url Ed25519 signature of the canonical payload below.</td></tr>
      </tbody></table>
      <CodeBlock>{canonical}</CodeBlock>
      <p><code>&lt;path-and-query&gt;</code> is the request URI beginning with <code>/</code>. <code>&lt;hex-sha256-body&gt;</code> is the lowercase hexadecimal SHA-256 digest of the exact request body bytes; for an empty body, hash zero bytes.</p>
    </DocSection>

    <DocSection title="Use an API key with the RC CLI">
      <CopyField value="RC_API_TOKEN='rcsk_...' ohrats-rc devices"/>
      <CopyField value="ohrats-rc devices --token 'rcsk_...'"/>
      <p>The CLI parses the <code>rcsk_</code> credential and signs requests locally. It does not send the private key as a bearer token.</p>
    </DocSection>

    <DocSection title="OpenAPI">
      <div className="public-doc-actions"><a className="header-link" href="/api/v1/openapi">Interactive reference <Arrow/></a><a className="header-link" href="/api/v1/openapi/json">OpenAPI JSON <Arrow/></a></div>
    </DocSection>
  </>, true);
}

export function cliDocsPage() {
  return page("cli", "CLI", "Installation, login, commands, local state, and transport security.", <>
    <DocSection title="Install">
      <CopyField value={`curl -fsSL ${PUBLIC_URL}/install.sh | sh`}/>
      <p>The installer downloads the signed binary for the current operating system and architecture to <code>~/.local/bin/ohrats-rc</code>. macOS and Linux are supported.</p>
      <p>For a new device, use the enrollment command generated by the Devices page. It includes the one-time enrollment token and installs the background service after enrollment.</p>
    </DocSection>

    <DocSection title="Sign in for human CLI use">
      <CopyField value="ohrats-rc login"/>
      <CopyField value="ohrats-rc login --expires 7d"/>
      <p><code>login</code> generates a local Ed25519 control key and opens RC for passkey approval. The default lifetime is until revoked. Finite values are <code>1h</code>, <code>1d</code>, <code>7d</code>, <code>30d</code>, <code>90d</code>, <code>180d</code>, and <code>1y</code>.</p>
      <p><code>ohrats-rc logout</code> revokes the hosted CLI session and removes the local account session.</p>
    </DocSection>

    <DocSection title="Commands">
      <table className="public-doc-table"><thead><tr><th>Command</th><th>Description</th></tr></thead><tbody>
        <tr><td><code>ohrats-rc devices</code></td><td>List devices, workspaces, online state, and Node version.</td></tr>
        <tr><td><code>ohrats-rc shell DEVICE</code></td><td>Open the machine's login shell in the current terminal.</td></tr>
        <tr><td><code>ohrats-rc run DEVICE -- CMD...</code></td><td>Run one command remotely and stream its output.</td></tr>
        <tr><td><code>ohrats-rc actions</code></td><td>List saved Actions.</td></tr>
        <tr><td><code>ohrats-rc action run ACTION --device DEVICE</code></td><td>Run a saved Action. Add <code>--confirm</code> when the Action requires confirmation.</td></tr>
        <tr><td><code>ohrats-rc status</code></td><td>Show local enrollment and current hosted device status.</td></tr>
        <tr><td><code>ohrats-rc enroll TOKEN</code></td><td>Enroll this machine. Optional flags: <code>--name</code>, <code>--url</code>, <code>--state-dir</code>.</td></tr>
        <tr><td><code>ohrats-rc service install|start|stop|status|uninstall</code></td><td>Manage the per-user background Node service.</td></tr>
        <tr><td><code>ohrats-rc update</code></td><td>Download and verify a signed newer Node release, then restart the service if installed.</td></tr>
        <tr><td><code>ohrats-rc device delete ID</code></td><td>Owner-authorized remote removal followed by hosted device deletion.</td></tr>
        <tr><td><code>ohrats-rc config show|path|set|unset</code></td><td>Read or modify the default RC server and enrollment name.</td></tr>
        <tr><td><code>ohrats-rc uninstall</code></td><td>Remove the service, unregister the device when possible, delete local RC state, and remove the installed binary.</td></tr>
      </tbody></table>
    </DocSection>

    <DocSection title="Examples">
      <CopyField value="ohrats-rc shell Mac"/>
      <CopyField value="ohrats-rc run Mac -- uname -a"/>
      <CopyField value="ohrats-rc action run deploy --device Mac --confirm"/>
    </DocSection>

    <DocSection title="Local state and transport security">
      <p>RC state is stored under <code>~/.config/ohrats-rc</code> by default. This includes device identity, RC Lock, device pins, CLI session data, and the CLI control private key.</p>
      <p>The CLI uses the same encrypted control protocol as the browser: a fresh X25519 session key, the pinned Node transport identity, a fresh Node ephemeral key, HKDF-SHA256, and AES-256-GCM. The hosted RC server relays ciphertext for CLI process control.</p>
    </DocSection>
  </>, true);
}
