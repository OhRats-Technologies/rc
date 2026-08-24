import { $, api, escapeHTML, relative } from '../api.js';
import { onRelayEvent } from '../events.js';

function detail(event) {
  if (event.detail?.name) return event.detail.name;
  if (event.detail?.command) return event.detail.command;
  if (event.detail?.deviceId) return event.detail.deviceId.slice(0, 8);
  if (event.detail?.processId) return event.detail.processId.slice(0, 8);
  if (event.device_id) return event.device_id.slice(0, 8);
  return '';
}

function row(event) {
  return `<div class="event-row"><span>${escapeHTML(event.kind.toUpperCase())}</span><span>${escapeHTML(detail(event))}</span><span>${relative(event.created_at || event.at)}</span></div>`;
}

export async function renderActivity(workspaceId) {
  const { workspace } = await api(`/api/v1/workspaces/${workspaceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / ACTIVITY</p><h1>Activity</h1></div></div>
      <div id="activity-list" class="event-list"></div>
    </section>`;
  const load = async () => {
    const { events } = await api(`/api/v1/workspaces/${workspaceId}/activity`);
    $('#activity-list').innerHTML = events.length ? events.map(row).join('') : '<div class="event-row"><span class="muted">NO ACTIVITY</span></div>';
  };
  await load();
  onRelayEvent(event => {
    if (event.kind === 'relay.connected') { load(); return; }
    if (!event.audit || event.workspaceId !== workspaceId) return;
    const list = $('#activity-list');
    if (list.querySelector('.muted')) list.innerHTML = '';
    list.insertAdjacentHTML('afterbegin', row(event));
    while (list.children.length > 100) list.lastElementChild.remove();
  });
}
