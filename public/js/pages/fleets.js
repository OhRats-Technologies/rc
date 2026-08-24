import { $, api, copyText, escapeHTML, relative } from '../api.js';

export async function renderFleets(workspaceId) {
  const { workspace, fleets } = await api(`/api/v1/workspaces/${workspaceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading">
        <div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / FLEETS</p><h1>Fleets</h1></div>
        ${['owner','member'].includes(workspace.role) ? `<a class="primary-button" href="/workspaces/${workspaceId}/fleets/new"><span class="ui-icon icon-add"></span>NEW FLEET</a>` : ''}
      </div>
      <div class="list">${fleets.length ? fleets.map(fleet => `
        <a class="list-row" href="/workspaces/${workspaceId}/fleets/${fleet.id}"><div><strong>${escapeHTML(fleet.name)}</strong><div class="meta">${fleet.device_count} ${fleet.device_count === 1 ? 'DEVICE' : 'DEVICES'}</div></div><span>→</span></a>
      `).join('') : '<div class="list-row"><span class="muted">No fleets.</span></div>'}</div>
    </section>`;
}

export async function renderNewFleet(workspaceId) {
  const { workspace } = await api(`/api/v1/workspaces/${workspaceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / FLEETS</p><h1>New fleet</h1></div></div>
      <form id="fleet-form" class="panel form-stack">
        <label>Name<input name="name" required autofocus></label>
        <button class="primary-button" type="submit">CREATE FLEET</button><p id="form-error" class="error"></p>
      </form>
    </section>`;
  $('#fleet-form').addEventListener('submit', async event => {
    event.preventDefault();
    try {
      const name = new FormData(event.currentTarget).get('name');
      const out = await api(`/api/v1/workspaces/${workspaceId}/fleets`, { method: 'POST', body: JSON.stringify({ name }) });
      location.href = `/workspaces/${workspaceId}/fleets/${out.id}`;
    } catch (error) { $('#form-error').textContent = error.message; }
  });
}

export async function renderFleet(workspaceId, fleetId) {
  const [{ workspace }, { fleet, devices }] = await Promise.all([
    api(`/api/v1/workspaces/${workspaceId}`), api(`/api/v1/fleets/${fleetId}`),
  ]);
  if (fleet.workspace_id !== workspaceId) throw new Error('Fleet not found');
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / FLEET</p><h1>${escapeHTML(fleet.name)}</h1></div></div>
      <section class="panel">
        <div class="page-heading"><div><p class="eyebrow">DEVICES</p></div>${['owner','member'].includes(workspace.role) ? '<button id="enroll-device" class="primary-button secondary-button" type="button">ENROLL DEVICE</button>' : ''}</div>
        <div class="list">${devices.length ? devices.map(device => `
          <a class="list-row" href="/devices/${device.id}"><div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div><span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : relative(device.last_seen)}</span></a>
        `).join('') : '<div class="list-row"><span class="muted">No devices in this fleet.</span></div>'}</div>
        <div id="enrollment-result" class="result" hidden></div>
      </section>
      ${['owner','member'].includes(workspace.role) ? `<section class="panel danger-zone"><p class="eyebrow">DELETE FLEET</p><a class="primary-button danger-button" href="/workspaces/${workspaceId}/fleets/${fleetId}/delete"><span class="ui-icon icon-trash"></span>DELETE FLEET</a></section>` : ''}
    </section>`;
  $('#enroll-device')?.addEventListener('click', async () => {
    const out = await api(`/api/v1/fleets/${fleetId}/enrollments`, { method: 'POST', body: '{}' });
    const result = $('#enrollment-result'); result.hidden = false;
    result.innerHTML = `<p class="muted">Run on the device. Expires in 24 hours.</p><pre>${escapeHTML(out.install)}</pre><button id="copy-enroll" class="text-button" type="button">COPY COMMAND</button>`;
    $('#copy-enroll').addEventListener('click', event => copyText(out.install, event.currentTarget));
  });
}

export async function renderDeleteFleet(workspaceId, fleetId) {
  const [{ workspace }, { fleet }] = await Promise.all([
    api(`/api/v1/workspaces/${workspaceId}`), api(`/api/v1/fleets/${fleetId}`),
  ]);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())} / FLEET</p><h1>Delete fleet</h1></div></div>
      <section class="panel danger-zone">
        <p>Delete <strong>${escapeHTML(fleet.name)}</strong>?</p>
        <p class="muted">Devices with no other fleet membership will need to be enrolled again.</p>
        <div class="page-actions"><button id="confirm-delete" class="primary-button danger-button" type="button"><span class="ui-icon icon-trash"></span>DELETE FLEET</button><a class="primary-button secondary-button" href="/workspaces/${workspaceId}/fleets/${fleetId}">CANCEL</a></div>
      </section>
    </section>`;
  $('#confirm-delete').addEventListener('click', async () => {
    await api(`/api/v1/fleets/${fleetId}`, { method: 'DELETE', body: '{}' });
    location.href = `/workspaces/${workspaceId}/fleets`;
  });
}
