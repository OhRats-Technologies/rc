import type { User } from "../../../src/core";
import type { McpScope } from "../../../src/mcp/types";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative, until } from "../format";
import { LifetimeSelect } from "../components";

type ConsentDevice = { id: string; name: string; workspace_name: string; role: string; online: boolean };

export function mcpAuthorizePage(user: User, requestId: string, clientName: string, callback: string, requestedScopes: McpScope[], devices: ConsentDevice[]) {
  const scope = (value: McpScope, title: string, copy: string, checked: boolean) => requestedScopes.includes(value) ?
    <label className="mcp-choice"><input type="checkbox" name="scope" value={value} defaultChecked={checked}/><span className="mcp-choice-copy"><strong>{title}</strong><small>{copy}</small></span></label> : null;
  return htmlDocument({ title: "Connect agent", scripts: ["mcp-authorize"], body:
    <section className="auth-shell" data-mcp-request={requestId}><div className="ohrats-grid auth-grid" aria-hidden="true"/><div className="auth-content mcp-consent">
      <p className="eyebrow">OHRATS RC / MCP</p><h1>Connect {clientName}</h1>
      <div className="mcp-identity"><span>Signed in as <strong>{user.name}</strong></span><button className="text-button" type="button" data-mcp-switch-account>NOT YOU?</button></div>
      <p className="page-copy">Choose exactly which machines and capabilities this AI agent may use.</p>
      <p className="meta">OAUTH CALLBACK · <code>{callback}</code></p>
      <form className="auth-form" data-mcp-form>
        <LifetimeSelect defaultValue="never"/>
        <fieldset className="scope-fields"><legend>Permissions</legend>
          {scope("mcp:observe", "Observe", "Machine status and metadata.", true)}
          {scope("mcp:actions", "Actions", "Run saved Actions captured by this grant.", true)}
          {scope("mcp:terminal", "Terminal", "Run arbitrary commands. Command and output plaintext pass through RC.", false)}
        </fieldset>
        <fieldset className="scope-fields"><legend>Machines</legend>{devices.length ? devices.map(device =>
          <label className="mcp-choice" key={device.id}><input type="checkbox" name="device" value={device.id}/><span className="mcp-choice-copy"><strong>{device.name}</strong><small>{device.workspace_name} · {device.role.toUpperCase()}{device.online ? " · ONLINE" : " · OFFLINE"}</small></span></label>)
          : <p className="empty-state">No machines are available.</p>}</fieldset>
        <div className="mcp-consent-actions"><button className="or-button" type="submit" disabled={!devices.length}>AUTHORIZE WITH PASSKEY</button><button className="or-button secondary" type="button" data-mcp-cancel>CANCEL</button></div>
        <p className="muted">Access can be revoked from RC at any time. OAuth access tokens remain short-lived and rotate independently.</p><p className="error" data-mcp-error/>
      </form>
    </div></section> });
}

export function mcpConnectionsPage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed", endpoint: string,
  grants: Array<{ record: { id: string; name: string; last_used: number | null; expires_at: number }; payload: { scopes: string[]; deviceIds: string[] } }>) {
  return htmlDocument({ title: "AI agents", user, workspaces, path: "/integrations/mcp", sidebar, scripts: ["mcp-page"], body:
    <div className="page"><header className="page-header"><div><h1>AI agents</h1><p className="page-copy">Connect an MCP client with this URL: <code>{endpoint}</code></p></div></header>
      <div className="settings-list">{grants.length ? grants.map(({ record, payload }) => <div className="setting-row" key={record.id}><div>
        <strong>{record.name}</strong><div className="meta">{payload.scopes.join(" · ").toUpperCase()} · {payload.deviceIds.length} MACHINE{payload.deviceIds.length === 1 ? "" : "S"} · {record.last_used ? `USED ${relative(record.last_used)}` : record.expires_at === 0 ? "UNTIL REVOKED" : `EXPIRES IN ${until(record.expires_at)}`}</div>
      </div><button className="text-button" type="button" data-mcp-revoke={record.id}>REVOKE</button></div>)
        : <p className="empty-state">No AI agents are connected yet. Paste the MCP URL into your agent to start.</p>}</div>
      <p className="error" data-mcp-page-error/>
    </div> });
}
