import type { User } from "../../src/core";
import { listDevices, nodeUpdateAvailable, type DeviceView } from "../../src/devices";
import { VERSION } from "../../src/config";
import type { WorkspaceView } from "../../src/workspaces";

function active(path: string, prefix: string) { return path.startsWith(prefix) ? " active" : ""; }

function DeviceItem({ device, owner, current, overflow, path }: {
  device: DeviceView; owner: boolean; current: boolean; overflow: boolean; path: string;
}) {
  const canUpdate = owner && device.capabilities.includes("update") && nodeUpdateAvailable(device.agent_version, VERSION);
  return <div className={`workspace-device-row${current ? " active" : ""}${overflow ? " workspace-device-overflow" : ""}`}
    data-sidebar-device={device.id} hidden={overflow}>
    <div className={`workspace-device-head${owner ? " has-menu" : ""}`}>
      <a className="workspace-device-link" href={`/devices/${device.id}`} data-device-name-view>
        <span className={`workspace-device-presence${device.online ? " online" : ""}`} data-sidebar-device-status={device.id}/>
        <span className="workspace-device-name"><span>{device.name}</span></span>
      </a>
      {owner && <form className="device-inline-rename" method="post" action={`/devices/${device.id}/rename`} hidden data-device-rename-form>
        <span className={`workspace-device-presence${device.online ? " online" : ""}`} data-sidebar-device-status={device.id}/>
        <input className="device-rename-input" name="name" defaultValue={device.name} aria-label={`Rename ${device.name}`} required maxLength={120}/>
        <input type="hidden" name="next" value={path}/>
      </form>}
      {owner && <details className="workspace-menu device-menu">
        <summary className="workspace-menu-trigger" aria-label={`Actions for ${device.name}`} title="Device actions"><span className="ui-icon icon-ellipsis" aria-hidden="true"/></summary>
        <div className="workspace-menu-popover">
          <div className="workspace-menu-actions">
            <button type="button" data-device-rename><span className="ui-icon icon-pencil" aria-hidden="true"/>Rename device</button>
            {canUpdate && <button type="button" data-sidebar-device-update={device.id} disabled={!device.online}>Update node</button>}
            <button className="danger-text" type="button" data-delete-kind="device" data-delete-name={device.name}
              data-delete-endpoint={`/api/v1/devices/${device.id}`}><span className="ui-icon icon-trash" aria-hidden="true"/>Delete device</button>
          </div>
        </div>
      </details>}
    </div>
  </div>;
}

function WorkspaceFolder({ workspace, devices, currentDeviceId, open, path }: {
  workspace: WorkspaceView; devices: DeviceView[]; currentDeviceId: string; open: boolean; path: string;
}) {
  const visible = devices.slice(0, 5), overflow = devices.slice(5);
  return <div className={`workspace-folder${open ? " active" : ""}`} data-workspace-folder={workspace.id} data-default-open={open ? "true" : "false"}>
    <div className="workspace-folder-head has-menu">
      <button className="workspace-toggle" type="button" aria-expanded={open} data-workspace-toggle={workspace.id} data-workspace-name-view>
        <span className="ui-icon icon-folder" aria-hidden="true"/><span className="workspace-name">{workspace.name}</span>
      </button>
      {workspace.role === "owner" && <form className="workspace-inline-rename" method="post" action={`/workspaces/${workspace.id}/rename`} hidden data-workspace-rename-form>
        <span className="ui-icon icon-folder" aria-hidden="true"/>
        <input className="workspace-rename-input" name="name" defaultValue={workspace.name} aria-label={`Rename ${workspace.name}`} required maxLength={120}/>
        <input type="hidden" name="next" value={path}/>
      </form>}
      <details className="workspace-menu">
        <summary className="workspace-menu-trigger" aria-label={`Actions for ${workspace.name}`} title="Workspace actions"><span className="ui-icon icon-ellipsis" aria-hidden="true"/></summary>
        <div className="workspace-menu-popover">
          <div className="workspace-menu-actions" data-workspace-menu-actions>
            {workspace.role === "owner" && <a href={`/devices/enroll?workspace=${workspace.id}`}><span className="ui-icon icon-enroll" aria-hidden="true"/>Enroll device</a>}
            <a href={`/actions?workspace=${workspace.id}`}><span className="ui-icon icon-bolt" aria-hidden="true"/>Actions</a>
            {workspace.role === "owner" && <a href={`/workspaces/${workspace.id}/access`}><span className="ui-icon icon-access" aria-hidden="true"/>Manage access</a>}
            <a href={`/workspaces/${workspace.id}/activity`}><span className="ui-icon icon-audit" aria-hidden="true"/>Audit log</a>
            {workspace.role === "owner" && <button type="button" data-workspace-rename><span className="ui-icon icon-pencil" aria-hidden="true"/>Rename workspace</button>}
            {workspace.role !== "owner" && <form method="post" action={`/workspaces/${workspace.id}/leave`}><button type="submit">Leave workspace</button></form>}
            {workspace.role === "owner" && <button className="danger-text" type="button" data-delete-kind="workspace" data-delete-name={workspace.name}
              data-delete-endpoint={`/api/v1/workspaces/${workspace.id}`}><span className="ui-icon icon-trash" aria-hidden="true"/>Delete workspace</button>}
          </div>
        </div>
      </details>
    </div>
    <div className="workspace-children" data-workspace-children={workspace.id} data-open={open ? "true" : "false"} hidden={!open}>
      {devices.length ? <>
        {[...visible, ...overflow].map((device, index) => <DeviceItem key={device.id} device={device} owner={workspace.role === "owner"}
          current={device.id === currentDeviceId} overflow={index >= 5} path={path}/>)}
        {overflow.length > 0 && <button className="workspace-show-more" type="button" data-workspace-show-more={workspace.id}>Show more</button>}
      </> : <span className="workspace-empty">No devices</span>}
    </div>
  </div>;
}

export function Sidebar({ user, workspaces, path }: { user: User; workspaces: WorkspaceView[]; path: string }) {
  const devices = listDevices(user), currentDeviceId = path.match(/^\/devices\/([^/]+)/)?.[1] || "";
  const workspaceId = path.match(/^\/workspaces\/([^/]+)/)?.[1] || devices.find(device => device.id === currentDeviceId)?.workspace_id || "";
  return <>
    <aside id="site-sidebar" className="site-sidebar">
      <div className="sidebar-scroll">
        <a className="site-brand" href="/devices"><img src="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg" alt=""/><strong>RC</strong></a>
        <nav aria-label="RC navigation">
          <section className="sidebar-section">
            <h2>Navigation</h2>
            <a className={`nav-link${active(path, "/devices")}`} href="/devices"><span className="ui-icon icon-devices"/><span>Devices</span></a>
            <a className={`nav-link${active(path, "/actions")}`} href="/actions"><span className="ui-icon icon-actions"/><span>Actions</span></a>
            <a className={`nav-link${path === "/api" ? " active" : ""}`} href="/api"><span className="ui-icon icon-api"/><span>API</span></a>
            <a className={`nav-link${active(path, "/integrations/mcp")}`} href="/integrations/mcp"><span className="ui-icon icon-api"/><span>MCP</span></a>
          </section>
          <section className="sidebar-section workspace-section">
            <div className="sidebar-section-title"><h2>Workspaces</h2><button className="workspace-add" type="button" aria-label="New workspace" title="New workspace" data-workspace-create-trigger><span className="ui-icon icon-plus" aria-hidden="true"/></button></div>
            <form className="workspace-create-form workspace-folder-head" method="post" action="/workspaces" hidden data-workspace-create-form>
              <span className="ui-icon icon-folder" aria-hidden="true"/>
              <input className="workspace-create-input" name="name" aria-label="Workspace name" required maxLength={120}/>
              <input type="hidden" name="next" value={path}/>
            </form>
            {workspaces.length ? workspaces.map(workspace => <WorkspaceFolder key={workspace.id} workspace={workspace}
              devices={devices.filter(device => device.workspace_id === workspace.id)} currentDeviceId={currentDeviceId} open={workspace.id === workspaceId} path={path}/>)
              : <span className="sidebar-empty" data-workspace-empty>NO WORKSPACES</span>}
          </section>
        </nav>
      </div>
      <div className="sidebar-footer"><div className="profile-row">
        <a className="profile-link" href="/account"><span className="profile-initial">{user.name.trim().slice(0, 1).toUpperCase() || "?"}</span><span className="profile-name">{user.name}</span></a>
        <button className="theme-toggle" type="button" data-theme-toggle aria-label="Toggle theme"/>
        <form method="post" action="/account/logout"><button className="icon-button" type="submit" aria-label="Sign out" title="Sign out"><span className="ui-icon icon-sign-out"/></button></form>
      </div></div>
    </aside>
    <dialog className="delete-dialog" data-delete-dialog aria-labelledby="delete-dialog-title">
      <div className="delete-dialog-content">
        <h2 id="delete-dialog-title" data-delete-title>Delete?</h2>
        <p>This will delete <strong data-delete-name>this item</strong>.</p>
        <p className="page-copy" data-delete-description hidden/>
        <p className="error" data-delete-error/>
        <div className="delete-dialog-actions">
          <button className="or-button secondary" type="button" data-delete-cancel>Cancel</button>
          <button className="or-button delete-confirm" type="button" data-delete-confirm>Delete</button>
        </div>
      </div>
    </dialog>
    <button id="sidebar-toggle" className="sidebar-toggle" type="button" aria-label="Toggle sidebar"><span className="ui-icon icon-sidebar"/></button>
  </>;
}
