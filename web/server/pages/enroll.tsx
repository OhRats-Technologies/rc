import type { User } from "../../../src/core";
import { listDevices } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";

export function enrollDevicePage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed", install = "", workspaceId = "", error = "") {
  const owners = workspaces.filter(workspace => workspace.role === "owner");
  const knownDevices = workspaceId ? listDevices(user).filter(device => device.workspace_id === workspaceId).map(device => device.id) : [];
  return htmlDocument({ title: "Enroll device", user, workspaces, path: "/devices/enroll", sidebar, scripts: install ? ["copy", "live"] : [], body:
    <div className="page narrow-form-page" data-enroll-page={workspaceId || undefined} data-known-devices={knownDevices.join(",")}><header className="page-header"><div><p className="eyebrow">DEVICES</p><h1>Enroll device</h1></div></header>
      {owners.length ? <form method="post" action="/devices/enroll" className="simple-form"><label>Workspace<select name="workspaceId">{owners.map(workspace => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}</select></label><button className="or-button" type="submit">CREATE INSTALL COMMAND</button></form> : <p className="empty-state">You need to own a workspace before enrolling a device.</p>}
      {install && <div className="enrollment-command"><span className="meta">INSTALL COMMAND · SHOWN ONCE</span><div className="or-copy-field" data-copy-value={install} title={install}><span className="or-copy-prefix">$</span><code>{install}</code><button className="or-copy-button copy-value" type="button" aria-label="Copy install command"><span className="or-copy-icon" aria-hidden="true"/></button></div><p id="enrollment-state" className="meta">Waiting for this device to connect…</p></div>}<p className="error">{error}</p>
    </div> });
}
