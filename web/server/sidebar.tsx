import type { User } from "../../src/core";
import type { WorkspaceView } from "../../src/workspaces";

function active(path: string, prefix: string) { return path.startsWith(prefix) ? " active" : ""; }

export function Sidebar({ user, workspaces, path }: { user: User; workspaces: WorkspaceView[]; path: string }) {
  const workspaceId = path.match(/^\/workspaces\/([^/]+)/)?.[1] || "";
  return <>
    <aside id="site-sidebar" className="site-sidebar">
      <div className="sidebar-scroll">
        <a className="site-brand" href="/devices"><img src="https://assets.ohrats.party/logo.svg" alt=""/><strong>Relay</strong></a>
        <nav aria-label="Relay navigation">
          <section className="sidebar-section">
            <h2>Navigation</h2>
            <a className={`nav-link${active(path, "/devices")}`} href="/devices"><span className="ui-icon icon-devices"/><span>Devices</span></a>
            <a className={`nav-link${active(path, "/workspaces")}`} href="/workspaces"><span className="ui-icon icon-workspaces"/><span>Workspaces</span></a>
            <a className={`nav-link${path === "/api" ? " active" : ""}`} href="/api"><span className="ui-icon icon-api"/><span>API</span></a>
          </section>
          <section className="sidebar-section">
            <h2>Workspaces</h2>
            {workspaces.length ? workspaces.map(workspace => <a key={workspace.id}
              className={`workspace-link${workspace.id === workspaceId ? " active" : ""}`}
              href={`/workspaces/${workspace.id}`}><span className="workspace-dot"/><span>{workspace.name}</span></a>)
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
