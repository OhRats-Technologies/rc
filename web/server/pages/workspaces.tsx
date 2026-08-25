import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { ActionView } from "../../../src/actions";
import type { ActivityView, WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative } from "../format";
import { SectionBadge } from "../components";

type WorkspaceResult =
  | { kind: "enrollment"; install: string };

function DeviceRows({ devices }: { devices: DeviceView[] }) {
  return <div id="workspace-device-list" className="data-list">{devices.length ? devices.map(device => <a key={device.id} className="data-row" href={`/devices/${device.id}`} data-device-row={device.id}>
    <div><strong>{device.name}</strong><div className="meta">{device.platform.toUpperCase()}/{device.arch}</div></div>
    <span className={`status${device.online ? " online" : ""}`} data-device-status={device.id}>{device.online ? "ONLINE" : relative(device.last_seen)}</span>
  </a>) : <p className="empty-state">No devices in this workspace.</p>}</div>;
}

export function workspacesPage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed") {
  return htmlDocument({ title: "Workspaces", user, workspaces, path: "/workspaces", sidebar, body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">WORKSPACES</p><h1>Workspaces</h1></div><a className="or-button" href="/workspaces/new">NEW WORKSPACE</a></header>
      <div className="data-list workspace-list">{workspaces.length ? workspaces.map((workspace, index) => <a className="data-row workspace-row" key={workspace.id} href={`/workspaces/${workspace.id}`}>
        <SectionBadge index={String(index + 1).padStart(2, "0")}>{workspace.name}</SectionBadge><span className="meta workspace-role">{workspace.role.toUpperCase()}</span>
      </a>) : <p className="empty-state">No workspaces.</p>}</div>
    </div> });
}

export function newWorkspacePage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed", error = "", value = "") {
  return htmlDocument({ title: "New workspace", user, workspaces, path: "/workspaces/new", sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">WORKSPACES</p><h1>New workspace</h1></div></header>
      <form method="post" action="/workspaces" className="simple-form"><label>Name<input name="name" defaultValue={value} required autoFocus/></label><button className="or-button" type="submit">CREATE</button><p className="error">{error}</p></form>
    </div> });
}

export function workspacePage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, devices: DeviceView[], actions: ActionView[], sidebar: "open" | "closed", result?: WorkspaceResult, error = "") {
  return htmlDocument({ title: workspace.name, user, workspaces, path: `/workspaces/${workspace.id}`, sidebar, scripts: ["live", "copy"], body:
    <div className="page" data-workspace-page={workspace.id}><header className="page-header"><div><p className="eyebrow">WORKSPACE</p><h1>{workspace.name}</h1><p className="meta">{workspace.role.toUpperCase()}</p></div></header>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="01">Devices</SectionBadge><h2>{devices.length} {devices.length === 1 ? "device" : "devices"}</h2></div></div><DeviceRows devices={devices}/>
        {workspace.role === "owner" && <form method="post" action={`/workspaces/${workspace.id}/enrollments`} className="inline-action-form"><button className="or-button" type="submit">ENROLL DEVICE</button></form>}
        {result?.kind === "enrollment" && <div className="enrollment-command"><span className="meta">INSTALL COMMAND</span><div className="or-copy-field" data-copy-value={result.install} title={result.install}><span className="or-copy-prefix">$</span><code>{result.install}</code><button className="or-copy-button copy-value" type="button" aria-label="Copy install command"><span className="or-copy-icon" aria-hidden="true"/></button></div></div>}
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="02">Actions</SectionBadge><h2>{actions.length} saved</h2></div>{workspace.role === "owner" && <a className="text-action" href={`/actions/new?workspace=${workspace.id}`}>NEW ACTION →</a>}</div>
        <div className="data-list">{actions.length ? actions.slice(0, 4).map(action => <a className="data-row" href={`/actions/${action.id}`} key={action.id}><div><strong>{action.name}</strong><div className="meta mono">{action.command}</div></div><span className="meta">OPEN →</span></a>) : <p className="empty-state">No saved actions.</p>}</div>
        {actions.length > 4 && <a className="text-action" href={`/actions?workspace=${workspace.id}`}>VIEW ALL →</a>}
      </section>
      <section className="content-section"><SectionBadge index="03">Activity</SectionBadge><a className="or-button" href={`/workspaces/${workspace.id}/activity`}>VIEW AUDIT LOG <span aria-hidden="true">→</span></a></section>
      {workspace.role === "owner" && <section className="content-section"><SectionBadge index="04">Access</SectionBadge><a className="or-button" href={`/workspaces/${workspace.id}/access`}>MANAGE ACCESS <span aria-hidden="true">→</span></a></section>}
      <section className="content-section danger-section"><div className="section-heading"><div><SectionBadge index={workspace.role === "owner" ? "05" : "04"}>Workspace</SectionBadge><h2>Settings</h2></div></div>
        <div className="actions"><form method="post" action={`/workspaces/${workspace.id}/leave`}><button className="text-action" type="submit">LEAVE WORKSPACE</button></form>{workspace.role === "owner" && <a className="text-action danger-text" href={`/workspaces/${workspace.id}/delete`}>DELETE WORKSPACE</a>}</div>
        <p className="error">{error}</p>
      </section>
    </div> });
}

export function deleteWorkspacePage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, sidebar: "open" | "closed", error = "") {
  return htmlDocument({ title: "Delete workspace", user, workspaces, path: `/workspaces/${workspace.id}/delete`, sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()}</p><h1>Delete workspace</h1></div></header>
      <section className="content-section danger-section"><p>Delete <strong>{workspace.name}</strong> and its devices?</p><form method="post" action={`/workspaces/${workspace.id}/delete`} className="actions"><button className="text-action danger-text" type="submit">DELETE</button><a className="text-action" href={`/workspaces/${workspace.id}`}>CANCEL</a></form><p className="error">{error}</p></section>
    </div> });
}

function eventDetail(event: ActivityView) {
  const label = String(event.detail.name || event.detail.command || event.detail.deviceId || event.detail.processId || event.device_id || "").slice(0, 120);
  const deviceId = String(event.detail.deviceId || event.device_id || ""), processId = String(event.detail.processId || ""), actionId = String(event.detail.actionId || "");
  if (deviceId && processId) return <a className="text-action activity-link" href={`/devices/${deviceId}/processes/${processId}`}>{label || processId.slice(0, 8)} →</a>;
  if (deviceId) return <a className="text-action activity-link" href={`/devices/${deviceId}`}>{label || deviceId.slice(0, 8)} →</a>;
  if (actionId) return <a className="text-action activity-link" href={`/actions/${actionId}`}>{label || actionId.slice(0, 8)} →</a>;
  return label;
}

export function activityPage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, events: ActivityView[], sidebar: "open" | "closed") {
  return htmlDocument({ title: "Activity", user, workspaces, path: `/workspaces/${workspace.id}/activity`, sidebar, scripts: ["live"], body:
    <div className="page" data-activity-page={workspace.id}><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()} / ACTIVITY</p><h1>Activity</h1></div></header>
      <section className="content-section"><div id="activity-list" className="activity-list">{events.length ? events.map(event => <div className="activity-row" key={event.id}><span>{event.kind.toUpperCase()}</span><span>{eventDetail(event)}</span><time>{relative(event.created_at)}</time></div>) : <p className="empty-state">No activity.</p>}</div></section>
    </div> });
}
