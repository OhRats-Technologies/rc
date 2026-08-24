import { $, api, escapeHTML, relative } from '../api.js';

function detail(event) {
  if (event.detail?.name) return event.detail.name;
  if (event.detail?.command) return event.detail.command;
  if (event.detail?.fleetId) return event.detail.fleetId.slice(0, 8);
  if (event.device_id) return event.device_id.slice(0, 8);
  return '';
}

export async function renderActivity(workspaceId) {
  const [{ workspace }, { events }] = await Promise.all([
    api(`/api/v1/workspaces/${workspaceId}`),
    api(`/api/v1/workspaces/${workspaceId}/activity`),
  ]);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / ACTIVITY</p><h1>Activity</h1></div></div>
      <div class="event-list">
        ${events.length ? events.map(event => `
          <div class="event-row"><span>${escapeHTML(event.kind.toUpperCase())}</span><span>${escapeHTML(detail(event))}</span><span>${relative(event.created_at)}</span></div>
        `).join('') : '<div class="event-row"><span class="muted">NO ACTIVITY</span></div>'}
      </div>
    </section>`;
}
