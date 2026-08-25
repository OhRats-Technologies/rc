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
    <div className="workspace-device-head">
      <a className="workspace-device-link" href={`/devices/${device.id}`} data-device-name-view>
        <span className={`workspace-device-presence${device.online ? " online" : ""}`} data-sidebar-device-status={device.id}/>
        <span className="workspace-device-name">{device.name}</span>
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
            <button type="button" data-device-rename>Rename device</button>
            {canUpdate && <button type="button" data-sidebar-device-update={device.id} disabled={!device.online}>Update node</button>}
            <a className="danger-text" href={`/devices/${device.id}/delete`}>Delete device</a>
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
    <div className="workspace-folder-head">
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
            {workspace.role === "owner" && <a href={`/workspaces/${workspace.id}/access`}><span className="ui-icon icon-share" aria-hidden="true"/>Share workspace</a>}
            {workspace.role === "owner" && <button type="button" data-workspace-rename>Rename workspace</button>}
            <a href={`/workspaces/${workspace.id}/activity`}>Audit log</a>
            {workspace.role === "owner" && <a className="danger-text" href={`/workspaces/${workspace.id}/delete`}>Delete workspace</a>}
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
            <a className={`nav-link${active(path, "/workspaces")}`} href="/workspaces"><span className="ui-icon icon-workspaces"/><span>Workspaces</span></a>
            <a className={`nav-link${active(path, "/actions")}`} href="/actions"><span className="ui-icon icon-actions"/><span>Actions</span></a>
            <a className={`nav-link${path === "/api" ? " active" : ""}`} href="/api"><span className="ui-icon icon-api"/><span>API</span></a>
          </section>
          <section className="sidebar-section workspace-section">
            <div className="sidebar-section-title"><h2>Workspaces</h2><a className="workspace-add" href="/workspaces/new" aria-label="New workspace" title="New workspace"><span className="ui-icon icon-plus" aria-hidden="true"/></a></div>
            {workspaces.length ? workspaces.map(workspace => <WorkspaceFolder key={workspace.id} workspace={workspace}
              devices={devices.filter(device => device.workspace_id === workspace.id)} currentDeviceId={currentDeviceId} open={workspace.id === workspaceId} path={path}/>)
              : <span className="sidebar-empty">NO WORKSPACES</span>}
          </section>
        </nav>
      </div>
      <div className="sidebar-footer"><div className="profile-row">
        <a className="profile-link" href="/account"><span className="profile-initial">{user.name.trim().slice(0, 1).toUpperCase() || "?"}</span><span className="profile-name">{user.name}</span></a>
        <button className="theme-toggle" type="button" data-theme-toggle aria-label="Toggle theme"/>
        <form method="post" action="/account/logout"><button className="icon-button" type="submit" aria-label="Sign out" title="Sign out"><span className="ui-icon icon-sign-out"/></button></form>
      </div></div>
    </aside>
    <button id="sidebar-toggle" className="sidebar-toggle" type="button" aria-label="Toggle sidebar"><span className="ui-icon icon-sidebar"/></button>
  </>;
}
