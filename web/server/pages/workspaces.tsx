import type { User } from "../../../src/core";
import type { ActivityView, WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative } from "../format";
import { SectionBadge } from "../components";

export function deleteWorkspacePage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, sidebar: "open" | "closed", error = "") {
  return htmlDocument({ title: "Delete workspace", user, workspaces, path: `/workspaces/${workspace.id}/delete`, sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()}</p><h1>Delete workspace</h1></div></header>
      <section className="content-section danger-section"><p>Delete <strong>{workspace.name}</strong> and its devices?</p><form method="post" action={`/workspaces/${workspace.id}/delete`} className="actions"><button className="text-action danger-text" type="submit">DELETE</button><a className="text-action" href="/devices">CANCEL</a></form><p className="error">{error}</p></section>
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
