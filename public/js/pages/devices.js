import { $, api, escapeHTML, relative } from '../api.js';
import { onRelayEvent, relayRequest } from '../events.js';

function deviceRows(devices) {
  return devices.length ? devices.map(device => `
    <a class="list-row" href="/devices/${device.id}">
      <div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.workspace_name)} · ${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div>
      <span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `SEEN ${relative(device.last_seen)}`}</span>
    </a>`).join('') : '<div class="list-row"><span class="muted">No devices yet. Enroll one from a workspace.</span></div>';
}

function processState(process) {
  if (process.status === 'starting') return 'STARTING';
  if (process.status === 'running') return 'RUNNING';
  if (process.status === 'lost') return 'LOST';
  return process.signal || `EXIT ${process.exit_code ?? '?'}`;
}

function processRows(deviceId, processes) {
  return processes.length ? processes.map(process => `
    <a class="list-row" href="/devices/${deviceId}/processes/${process.id}">
      <div><strong class="mono">${escapeHTML(process.command)}</strong><div class="meta">${escapeHTML(process.cwd || '~')} · ${relative(process.created_at)}</div></div>
      <span class="status ${process.status === 'running' ? 'online' : ''}">${escapeHTML(processState(process))}</span>
    </a>`).join('') : '<div class="list-row"><span class="muted">No processes yet.</span></div>';
}

export async function renderDevices() {
  const { devices } = await api('/api/v1/devices');
  $('#page').innerHTML = `<section class="page"><div class="page-heading"><div><p class="eyebrow">DEVICES</p><h1>Devices</h1></div></div><div id="device-list" class="list">${deviceRows(devices)}</div></section>`;
  const refresh = () => api('/api/v1/devices').then(data => { $('#device-list').innerHTML = deviceRows(data.devices); }).catch(() => {});
  onRelayEvent(event => { if (event.kind === 'relay.connected' || event.kind.startsWith('device.')) refresh(); });
}

export async function renderDevice(deviceId) {
  const [{ device }, { processes }, status] = await Promise.all([
    api(`/api/v1/devices/${deviceId}`), api(`/api/v1/devices/${deviceId}/processes`), api('/api/v1/status'),
  ]);
  const supportsProcess = device.capabilities.includes('process'), supportsUpdate = device.capabilities.includes('update');
  $('#page').innerHTML = `<section class="page">
    <div class="page-heading">
      <div><p class="eyebrow">DEVICE</p><h1>${escapeHTML(device.name)}</h1><div class="meta"><a href="/workspaces/${device.workspace_id}">${escapeHTML(device.workspace_name)}</a> · ${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.hostname)}</div></div>
      <span id="device-status" class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `LAST SEEN ${relative(device.last_seen)}`}</span>
    </div>
    <div class="device-overview-grid">
      <section class="panel">
        <div class="page-heading compact"><div><p class="eyebrow">NEW PROCESS</p></div></div>
        <form id="process-launch" class="process-launch-form">
          <label>Working directory<input id="process-cwd" spellcheck="false" placeholder="~"></label>
          <label>Command<input id="process-command" spellcheck="false" value="sh" required></label>
          <button class="primary-button" type="submit" ${device.online && supportsProcess ? '' : 'disabled'}>START PROCESS</button>
        </form>
        <p id="process-error" class="error">${supportsProcess ? '' : 'Update this node to use processes.'}</p>
      </section>
      <aside class="node-inspector panel">
        <p class="eyebrow">NODE</p><dl class="fact-list">
          <div><dt>HOST</dt><dd>${escapeHTML(device.hostname)}</dd></div><div><dt>OS</dt><dd>${escapeHTML(device.platform.toUpperCase())}</dd></div>
          <div><dt>ARCH</dt><dd>${escapeHTML(device.arch)}</dd></div><div><dt>NODE</dt><dd id="node-agent">${escapeHTML(device.agent_version)}</dd></div>
          <div><dt>RELAY</dt><dd>${escapeHTML(status.version)}</dd></div>
        </dl>
        <button id="update-node" class="primary-button secondary-button inspector-action" type="button" ${device.online && supportsUpdate ? '' : 'disabled'}>${supportsUpdate ? 'UPDATE NODE' : 'CLI UPDATE REQUIRED'}</button>
        <p id="update-state" class="meta">${supportsUpdate ? 'Stops running processes.' : 'curl -fsSL https://relay.ohrats.party/install.sh | sh'}</p>
      </aside>
    </div>
    <section class="panel process-list-panel">
      <div class="page-heading compact"><div><p class="eyebrow">PROCESSES</p><h2>Process history</h2></div></div>
      <div id="process-list" class="list">${processRows(deviceId, processes)}</div>
    </section>
  </section>`;

  async function refreshDevice() {
    const [{ device: current }, { processes: currentProcesses }] = await Promise.all([
      api(`/api/v1/devices/${deviceId}`), api(`/api/v1/devices/${deviceId}/processes`),
    ]);
    const badge = $('#device-status'); badge.classList.toggle('online', current.online);
    badge.textContent = current.online ? 'ONLINE' : `LAST SEEN ${relative(current.last_seen)}`;
    $('#node-agent').textContent = current.agent_version;
    const canProcess = current.online && current.capabilities.includes('process');
    const canUpdate = current.online && current.capabilities.includes('update');
    $('#process-launch button').disabled = !canProcess;
    const updateButton = $('#update-node'); updateButton.disabled = !canUpdate;
    updateButton.textContent = current.capabilities.includes('update') ? 'UPDATE NODE' : 'CLI UPDATE REQUIRED';
    $('#update-state').textContent = current.capabilities.includes('update') ? 'Stops running processes.' : 'curl -fsSL https://relay.ohrats.party/install.sh | sh';
    $('#process-list').innerHTML = processRows(deviceId, currentProcesses);
  }

  $('#process-launch').addEventListener('submit', async event => {
    event.preventDefault(); $('#process-error').textContent = '';
    const command = $('#process-command').value.trim(), cwd = $('#process-cwd').value.trim();
    try {
      const result = await relayRequest('process.start', { deviceId, command, cwd, cols: 100, rows: 30 });
      location.href = `/devices/${deviceId}/processes/${result.processId}`;
    } catch (error) { $('#process-error').textContent = error.message; }
  });

  $('#update-node').addEventListener('click', async event => {
    const button = event.currentTarget; button.disabled = true; $('#update-state').textContent = 'Starting update…';
    try { await relayRequest('node.update', { deviceId }); $('#update-state').textContent = 'Updating and restarting…'; }
    catch (error) { $('#update-state').textContent = error.message; button.disabled = false; }
  });

  onRelayEvent(event => {
    if (event.kind === 'relay.connected') { refreshDevice().catch(() => {}); return; }
    if (event.deviceId !== deviceId) return;
    if (event.kind === 'node.update.error') { $('#update-state').textContent = event.detail?.error || 'Update failed.'; refreshDevice().catch(() => {}); return; }
    if (event.kind === 'node.update.ready') { $('#update-state').textContent = 'Restarting node…'; return; }
    if (event.kind.startsWith('device.') || event.kind.startsWith('process.')) refreshDevice().catch(() => {});
  });
}
