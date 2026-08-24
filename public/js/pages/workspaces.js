import { $, api, copyText, escapeHTML, relative } from '../api.js';
import { onRelayEvent } from '../events.js';

export async function renderWorkspaces() {
  const { workspaces } = await api('/api/v1/workspaces');
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading">
        <div><p class="eyebrow">WORKSPACES</p><h1>Workspaces</h1></div>
        <a class="primary-button" href="/workspaces/new"><span class="ui-icon icon-add"></span>NEW WORKSPACE</a>
      </div>
      <div class="list">${workspaces.length ? workspaces.map(workspace => `
        <a class="list-row" href="/workspaces/${workspace.id}"><div><strong>${escapeHTML(workspace.name)}</strong><div class="meta">${escapeHTML(workspace.role.toUpperCase())}</div></div><span>→</span></a>
      `).join('') : '<div class="list-row"><span class="muted">No workspaces.</span></div>'}</div>
    </section>`;
}

export function renderNewWorkspace() {
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">WORKSPACES</p><h1>New workspace</h1></div></div>
      <form id="workspace-form" class="panel form-stack">
        <label>Name<input name="name" required autofocus></label>
        <button class="primary-button" type="submit">CREATE WORKSPACE</button>
        <p id="form-error" class="error"></p>
      </form>
    </section>`;
  $('#workspace-form').addEventListener('submit', async event => {
    event.preventDefault();
    try {
      const name = new FormData(event.currentTarget).get('name');
      const out = await api('/api/v1/workspaces', { method: 'POST', body: JSON.stringify({ name }) });
      location.href = `/workspaces/${out.id}`;
    } catch (error) { $('#form-error').textContent = error.message; }
  });
}

export async function renderWorkspace(workspaceId) {
  const data = await api(`/api/v1/workspaces/${workspaceId}`), { workspace, devices } = data;
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">WORKSPACE</p><h1>${escapeHTML(workspace.name)}</h1><div class="meta">${escapeHTML(workspace.role.toUpperCase())}</div></div></div>
      <section class="panel">
        <div class="page-heading compact"><div><p class="eyebrow">DEVICES</p><h2 id="workspace-device-count">${devices.length} ${devices.length === 1 ? 'device' : 'devices'}</h2></div>${['owner','member'].includes(workspace.role) ? '<button id="enroll-device" class="primary-button secondary-button" type="button">ENROLL DEVICE</button>' : ''}</div>
        <div id="workspace-device-list" class="list">${devices.length ? devices.map(device => `
          <a class="list-row" href="/devices/${device.id}"><div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div><span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : relative(device.last_seen)}</span></a>
        `).join('') : '<div class="list-row"><span class="muted">No devices in this workspace.</span></div>'}</div>
        <div id="enrollment-result" class="result" hidden></div>
      </section>
      <div class="card-grid"><a class="card" href="/workspaces/${workspaceId}/activity"><p class="eyebrow">ACTIVITY</p><h2>Audit log</h2><span class="muted">Workspace events.</span></a></div>
      ${workspace.role === 'owner' ? `
      <section class="panel">
        <p class="eyebrow">INVITE</p><p class="muted">Create a one-use workspace invite.</p>
        <button id="create-invite" class="primary-button secondary-button" type="button">CREATE INVITE</button>
        <div id="invite-result" class="result" hidden></div>
      </section>
      <section class="panel danger-zone">
        <p class="eyebrow">DELETE WORKSPACE</p><p class="muted">Deletes this workspace and its devices.</p>
        <a class="primary-button danger-button" href="/workspaces/${workspaceId}/delete"><span class="ui-icon icon-trash"></span>DELETE WORKSPACE</a>
      </section>` : ''}
    </section>`;
  const refreshDevices = async () => {
    const data = await api(`/api/v1/workspaces/${workspaceId}`);
    $('#workspace-device-count').textContent = `${data.devices.length} ${data.devices.length === 1 ? 'device' : 'devices'}`;
    $('#workspace-device-list').innerHTML = data.devices.length ? data.devices.map(device => `
      <a class="list-row" href="/devices/${device.id}"><div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div><span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : relative(device.last_seen)}</span></a>
    `).join('') : '<div class="list-row"><span class="muted">No devices in this workspace.</span></div>';
  };
  onRelayEvent(event => {
    if (event.workspaceId !== workspaceId) return;
    if (event.kind.startsWith('device.')) refreshDevices().catch(() => {});
  });
  $('#enroll-device')?.addEventListener('click', async () => {
    const out = await api(`/api/v1/workspaces/${workspaceId}/enrollments`, { method: 'POST', body: '{}' });
    const result = $('#enrollment-result'); result.hidden = false;
    result.innerHTML = `<p class="muted">Run on the device. Expires in 24 hours.</p><pre>${escapeHTML(out.install)}</pre><button id="copy-enroll" class="text-button" type="button">COPY COMMAND</button>`;
    $('#copy-enroll').addEventListener('click', event => copyText(out.install, event.currentTarget));
  });
  if (workspace.role !== 'owner') return;
  $('#create-invite').addEventListener('click', async () => {
    const out = await api(`/api/v1/workspaces/${workspaceId}/invites`, { method: 'POST', body: JSON.stringify({ role: 'member' }) });
    const result = $('#invite-result'); result.hidden = false;
    result.innerHTML = `<pre>${escapeHTML(out.url)}</pre><button id="copy-invite" class="text-button" type="button">COPY INVITE</button>`;
    $('#copy-invite').addEventListener('click', event => copyText(out.url, event.currentTarget));
  });
}

export async function renderDeleteWorkspace(workspaceId) {
  const { workspace } = await api(`/api/v1/workspaces/${workspaceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())}</p><h1>Delete workspace</h1></div></div>
      <section class="panel danger-zone">
        <p>Delete <strong>${escapeHTML(workspace.name)}</strong>?</p>
        <p class="muted">This also removes every device in the workspace.</p>
        <div class="page-actions"><button id="confirm-delete" class="primary-button danger-button" type="button"><span class="ui-icon icon-trash"></span>DELETE WORKSPACE</button><a class="primary-button secondary-button" href="/workspaces/${workspaceId}">CANCEL</a></div>
      </section>
    </section>`;
  $('#confirm-delete').addEventListener('click', async () => {
    await api(`/api/v1/workspaces/${workspaceId}`, { method: 'DELETE', body: '{}' });
    location.href = '/workspaces';
  });
}
