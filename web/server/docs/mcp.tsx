import { CodeBlock, CopyField, DocTable } from "./components";
import type { DocArticle } from "./types";
import { PUBLIC_URL } from "../../../src/config";

export function mcpArticle(): DocArticle {
  const endpoint = `${PUBLIC_URL}/mcp`;
  const cursor = `{
  "mcpServers": {
    "rc": { "url": "${endpoint}" }
  }
}`;
  const vscode = `{
  "servers": {
    "rc": { "type": "http", "url": "${endpoint}" }
  }
}`;
  const agy = `{
  "mcpServers": {
    "rc": { "serverUrl": "${endpoint}" }
  }
}`;
  return {
    slug: "mcp",
    title: "MCP",
    intro: "Configure an MCP client, authorize machines and scopes, and use RC tools.",
    copy: true,
    sections: [
      {
        id: "endpoint",
        title: "Endpoint",
        body: <>
          <CopyField value={endpoint} prefix=""/>
          <p>All users connect to the same endpoint and authenticate with OAuth. The MCP server name and configuration identifier are <strong>RC</strong> and <code>rc</code>.</p>
        </>,
      },
      {
        id: "codex",
        title: "Codex",
        body: <>
          <CopyField value={`codex mcp add rc --url ${endpoint} --oauth-resource ${endpoint}`}/>
          <CopyField value="codex mcp login --scopes mcp:observe,mcp:terminal rc"/>
          <p>Request only the scopes the agent needs. Omit <code>mcp:terminal</code> for read-only access.</p>
        </>,
      },
      {
        id: "claude-code",
        title: "Claude Code",
        body: <>
          <CopyField value={`claude mcp add --scope user --transport http rc ${endpoint}`}/>
          <CopyField value="claude mcp login rc"/>
          <p>Use <code>/mcp</code> inside Claude Code to inspect configured servers and authentication state.</p>
        </>,
      },
      {
        id: "cursor",
        title: "Cursor",
        body: <>
          <p>Add the server to <code>~/.cursor/mcp.json</code> for all projects or <code>.cursor/mcp.json</code> for one project.</p>
          <CodeBlock>{cursor}</CodeBlock>
          <CopyField value="cursor-agent mcp login rc"/>
        </>,
      },
      {
        id: "vscode",
        title: "Visual Studio Code",
        body: <>
          <p>Add the server to the user MCP configuration or <code>.vscode/mcp.json</code> in a workspace.</p>
          <CodeBlock>{vscode}</CodeBlock>
          <p>Start the server from <strong>MCP: List Servers</strong> or use it from Agent mode. VS Code opens browser authentication for remote MCP servers.</p>
        </>,
      },
      {
        id: "antigravity",
        title: "Antigravity CLI (agy)",
        body: <>
          <p>Add the server to <code>~/.gemini/config/mcp_config.json</code>, or use <code>.agents/mcp_config.json</code> for one workspace.</p>
          <CodeBlock>{agy}</CodeBlock>
          <p>Start <code>agy</code> and use <code>/mcp</code> to inspect configured servers and authenticate when required.</p>
        </>,
      },
      {
        id: "authorization",
        title: "Authorize RC",
        body: <>
          <p>The MCP client opens RC in the browser. RC shows the client name and callback URI and asks you to choose machines, scopes, and grant lifetime.</p>
          <p><strong>Terminal</strong> is not selected by default. Execution-capable grants require an Owner and a fresh passkey ceremony.</p>
          <p>The default grant lifetime is until revoked. Finite choices are 1 hour, 1 day, 7 days, 30 days, 90 days, 180 days, and 1 year.</p>
        </>,
      },
      {
        id: "scopes-tools",
        title: "Scopes and tools",
        body: <>
          <DocTable><thead><tr><th>Scope</th><th>Tools</th><th>Meaning</th></tr></thead><tbody>
            <tr><td><code>mcp:observe</code></td><td><code>machines_list</code>, <code>process_status</code></td><td>Read granted machine metadata and status/output for MCP processes created by the same grant.</td></tr>
            <tr><td><code>mcp:terminal</code></td><td><code>process_run</code></td><td>Run an arbitrary shell command on an explicitly granted machine.</td></tr>
          </tbody></DocTable>
        </>,
      },
      {
        id: "oauth-node-verification",
        title: "OAuth and Node verification",
        body: <>
          <p>RC publishes Protected Resource Metadata and authorization-server metadata. Clients use PKCE S256 and resource-bound access tokens. Access tokens are short-lived; refresh tokens rotate and cannot outlive the durable grant.</p>
          <p>The browser control identity signs the exact MCP grant. RC synchronizes its hash into RC Lock.</p>
          <p>Before execution, the Node verifies the grant signature, current Owner signer, selected device, required scope, grant presence in RC Lock, and expiry.</p>
        </>,
      },
      {
        id: "long-running-processes",
        title: "Long-running processes",
        body: <>
          <p><code>process_run</code> waits up to the requested timeout and returns output plus process status. If the process is still running, call <code>process_status</code> with the process ID and returned <code>nextOffset</code>.</p>
          <p><code>process_status</code> returns only bytes after the requested offset and can wait up to 60 seconds for new output or exit.</p>
        </>,
      },
      {
        id: "terminal-data",
        title: "Terminal data handling",
        body: <>
          <p>MCP Terminal command/output plaintext passes through bounded RC server memory because standard remote MCP does not use RC's browser/CLI end-to-end transport.</p>
          <p>The buffer is capped at 256 KiB and is not persisted to SQLite. Completed buffers expire after five minutes; inactive running buffers expire after thirty minutes.</p>
        </>,
      },
    ],
  };
}
