import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { ActivityView, WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative } from "../format";

function DeviceRows({ devices }: { devices: DeviceView[] }) {
  return <div id="workspace-device-list" className="data-list">{devices.length ? devices.map(device => <a key={device.id} className="data-row" href={`/devices/${device.id}`} data-device-row={device.id}>
    <div><strong>{device.name}</strong><div className="meta">{device.platform.toUpperCase()}/{device.arch}</div></div>
    <span className={`status${device.online ? " online" : ""}`} data-device-status={device.id}>{device.online ? "ONLINE" : relative(device.last_seen)}</span>
  </a>) : <p className="empty-state">No devices in this workspace.</p>}</div>;
}

export function workspacesPage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed") {
  return htmlDocument({ title: "Workspaces", user, workspaces, path: "/workspaces", sidebar, body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">WORKSPACES</p><h1>Workspaces</h1></div><a className="text-action" href="/workspaces/new">NEW WORKSPACE</a></header>
      <div className="data-list">{workspaces.length ? workspaces.map(workspace => <a className="data-row" key={workspace.id} href={`/workspaces/${workspace.id}`}><div><strong>{workspace.name}</strong><div className="meta">{workspace.role.toUpperCase()}</div></div><span>→</span></a>) : <p className="empty-state">No workspaces.</p>}</div>
    </div> });
}

export function newWorkspacePage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed", error = "", value = "") {
  return htmlDocument({ title: "New workspace", user, workspaces, path: "/workspaces/new", sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">WORKSPACES</p><h1>New workspace</h1></div></header>
      <form method="post" action="/workspaces" className="simple-form"><label>Name<input name="name" defaultValue={value} required autoFocus/></label><button className="primary-button" type="submit">CREATE</button><p className="error">{error}</p></form>
    </div> });
}

export function workspacePage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, devices: DeviceView[], sidebar: "open" | "closed", result?: { kind: string; value: string }) {
  const writable = workspace.role === "owner" || workspace.role === "member";
  return htmlDocument({ title: workspace.name, user, workspaces, path: `/workspaces/${workspace.id}`, sidebar, scripts: ["live", "workspace"], body:
    <div className="page" data-workspace-page={workspace.id}><header className="page-header"><div><p className="eyebrow">WORKSPACE</p><h1>{workspace.name}</h1><p className="meta">{workspace.role.toUpperCase()}</p></div></header>
      <section className="content-section"><div className="section-heading"><div><p className="eyebrow">DEVICES</p><h2>{devices.length} {devices.length === 1 ? "device" : "devices"}</h2></div></div><DeviceRows devices={devices}/>
        {result && <div className="credential-result" data-copy-value={result.value}><span className="meta">{result.kind.toUpperCase()}</span><code>{result.value}</code><button className="text-button copy-value" type="button">COPY</button></div>}
        {writable && <form method="post" action={`/workspaces/${workspace.id}/enrollments`} className="inline-action-form"><button className="text-button" type="submit">ENROLL DEVICE</button></form>}
      </section>
      <section className="content-section"><a className="section-link" href={`/workspaces/${workspace.id}/activity`}><span><span className="eyebrow">ACTIVITY</span><strong>Audit log</strong></span><span>→</span></a></section>
      {workspace.role === "owner" && <><section className="content-section"><div className="section-heading"><div><p className="eyebrow">INVITE</p><h2>Workspace access</h2></div></div><form method="post" action={`/workspaces/${workspace.id}/invites`} className="inline-action-form"><input type="hidden" name="role" value="member"/><button className="text-button" type="submit">CREATE INVITE</button></form></section>
      <section className="content-section danger-section"><p className="eyebrow">DELETE WORKSPACE</p><a className="text-action danger-text" href={`/workspaces/${workspace.id}/delete`}>DELETE WORKSPACE</a></section></>}
    </div> });
}

export function deleteWorkspacePage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, sidebar: "open" | "closed", error = "") {
  return htmlDocument({ title: "Delete workspace", user, workspaces, path: `/workspaces/${workspace.id}/delete`, sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()}</p><h1>Delete workspace</h1></div></header>
      <section className="content-section danger-section"><p>Delete <strong>{workspace.name}</strong> and its devices?</p><form method="post" action={`/workspaces/${workspace.id}/delete`} className="actions"><button className="text-action danger-text" type="submit">DELETE</button><a className="text-action" href={`/workspaces/${workspace.id}`}>CANCEL</a></form><p className="error">{error}</p></section>
    </div> });
}

function detail(event: ActivityView) {
  if (event.detail.name) return String(event.detail.name); if (event.detail.command) return String(event.detail.command);
  if (event.detail.deviceId) return String(event.detail.deviceId).slice(0, 8); if (event.detail.processId) return String(event.detail.processId).slice(0, 8);
  return event.device_id?.slice(0, 8) || "";
}

export function activityPage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, events: ActivityView[], sidebar: "open" | "closed") {
  return htmlDocument({ title: "Activity", user, workspaces, path: `/workspaces/${workspace.id}/activity`, sidebar, scripts: ["live"], body:
    <div className="page" data-activity-page={workspace.id}><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()} / ACTIVITY</p><h1>Activity</h1></div></header>
      <section className="content-section"><div id="activity-list" className="activity-list">{events.length ? events.map(event => <div className="activity-row" key={event.id}><span>{event.kind.toUpperCase()}</span><span>{detail(event)}</span><time>{relative(event.created_at)}</time></div>) : <p className="empty-state">No activity.</p>}</div></section>
    </div> });
}
