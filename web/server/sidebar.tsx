import type { User } from "../../src/core";
import { listDevices, type DeviceView } from "../../src/devices";
import type { WorkspaceView } from "../../src/workspaces";

function active(path: string, prefix: string) { return path.startsWith(prefix) ? " active" : ""; }

function WorkspaceFolder({ workspace, devices, currentDeviceId, open }: {
  workspace: WorkspaceView; devices: DeviceView[]; currentDeviceId: string; open: boolean;
}) {
  const visible = devices.slice(0, 5), overflow = devices.slice(5);
  return <div className={`workspace-folder${open ? " active" : ""}`} data-workspace-folder={workspace.id} data-default-open={open ? "true" : "false"}>
    <div className="workspace-folder-head">
      <button className="workspace-toggle" type="button" aria-expanded={open} data-workspace-toggle={workspace.id}>
        <span className="ui-icon icon-folder" aria-hidden="true"/><span className="workspace-name">{workspace.name}</span>
      </button>
      <details className="workspace-menu">
        <summary className="workspace-menu-trigger" aria-label={`Actions for ${workspace.name}`} title="Workspace actions"><span className="ui-icon icon-ellipsis" aria-hidden="true"/></summary>
        <div className="workspace-menu-popover">
          {workspace.role === "owner" && <a href={`/workspaces/${workspace.id}/access`}><span className="ui-icon icon-share" aria-hidden="true"/>Share workspace</a>}
          {workspace.role === "owner" && <a href={`/workspaces/${workspace.id}/rename`}>Rename workspace</a>}
          <a href={`/workspaces/${workspace.id}/activity`}>Audit log</a>
          {workspace.role === "owner" && <a className="danger-text" href={`/workspaces/${workspace.id}/delete`}>Delete workspace</a>}
        </div>
      </details>
    </div>
    <div className="workspace-children" data-workspace-children={workspace.id} hidden={!open}>
      {devices.length ? <>
        {[...visible, ...overflow].map((device, index) => <a key={device.id} className={`workspace-device${device.id === currentDeviceId ? " active" : ""}${index >= 5 ? " workspace-device-overflow" : ""}`} href={`/devices/${device.id}`} hidden={index >= 5}>
          <span className={`workspace-device-presence${device.online ? " online" : ""}`} data-sidebar-device-status={device.id}/><span>{device.name}</span>
        </a>)}
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
              devices={devices.filter(device => device.workspace_id === workspace.id)} currentDeviceId={currentDeviceId} open={workspace.id === workspaceId}/>)
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
