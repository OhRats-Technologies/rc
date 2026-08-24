import { api, escapeHTML, qs, relative } from "../api";
import { onRelayEvent } from "../events";
import type { RelayEvent, Workspace } from "../types";

function detail(event: RelayEvent) {
  const value = event.detail || {};
  if (value.name) return String(value.name);
  if (value.command) return String(value.command);
  if (value.deviceId) return String(value.deviceId).slice(0, 8);
  if (value.processId) return String(value.processId).slice(0, 8);
  if (event.device_id) return event.device_id.slice(0, 8);
  return "";
}

function row(event: RelayEvent) {
  return `<div class="activity-row"><span>${escapeHTML(event.kind.toUpperCase())}</span><span>${escapeHTML(detail(event))}</span><time>${relative(event.created_at || event.at)}</time></div>`;
}

export async function renderActivity(workspaceId: string) {
  const { workspace } = await api<{ workspace: Workspace }>(`/api/v1/workspaces/${workspaceId}`);
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / ACTIVITY</p><h1>Activity</h1></div></header>
    <section class="content-section"><div id="activity-list" class="activity-list"></div></section>
  </div>`;
  const load = async () => {
    const { events } = await api<{ events: RelayEvent[] }>(`/api/v1/workspaces/${workspaceId}/activity`);
    qs<HTMLElement>("#activity-list").innerHTML = events.length ? events.map(row).join("") : '<p class="empty-state">No activity.</p>';
  };
  await load();
  return onRelayEvent(event => {
    if (event.kind === "relay.connected") { void load(); return; }
    if (!event.audit || event.workspaceId !== workspaceId) return;
    const list = qs<HTMLElement>("#activity-list");
    list.querySelector(".empty-state")?.remove();
    list.insertAdjacentHTML("afterbegin", row(event));
    while (list.children.length > 100) list.lastElementChild?.remove();
  });
}
