import { $, api, escapeHTML, relative } from '../api.js';

export async function renderDevices() {
  const { devices } = await api('/api/v1/devices');
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">DEVICES</p><h1>Devices</h1></div></div>
      <div class="list">
        ${devices.length ? devices.map(device => `
          <a class="list-row" href="/devices/${device.id}">
            <div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.workspaces || '')}</div></div>
            <span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `SEEN ${relative(device.last_seen)}`}</span>
          </a>`).join('') : '<div class="list-row"><span class="muted">No devices yet. Enroll one from a fleet.</span></div>'}
      </div>
    </section>`;
}

function jobsHTML(jobs) {
  return jobs.length ? jobs.map(job => `
    <div class="job">
      <div class="job-command">$ ${escapeHTML(job.payload.command)}</div>
      <div class="job-result">${job.result == null ? `[${escapeHTML(job.status)}]` : escapeHTML(job.result)}${job.exit_code != null && job.exit_code !== 0 ? `\n[exit ${job.exit_code}]` : ''}</div>
    </div>`).join('') : '<span class="muted">Session ready.</span>';
}

export async function renderDevice(deviceId) {
  const { device } = await api(`/api/v1/devices/${deviceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading">
        <div><p class="eyebrow">DEVICE</p><h1>${escapeHTML(device.name)}</h1><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.hostname)}</div></div>
        <span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `LAST SEEN ${relative(device.last_seen)}`}</span>
      </div>
      <section class="panel">
        <p class="eyebrow">FLEETS</p>
        <div class="meta">${device.fleets.map(fleet => `${escapeHTML(fleet.workspace_name)} / ${escapeHTML(fleet.name)}`).join(' · ') || 'NONE'}</div>
      </section>
      <section class="panel">
        <p class="eyebrow">SHELL</p>
        <div id="terminal" class="terminal"><span class="muted">${device.online ? 'Session ready.' : 'Device is offline.'}</span></div>
        <form id="command-form" class="command-row">
          <input id="command" autocomplete="off" spellcheck="false" placeholder="uname -a" ${device.online ? '' : 'disabled'} required>
          <button class="primary-button" type="submit" ${device.online ? '' : 'disabled'}>RUN</button>
        </form>
      </section>
    </section>`;
  if (!device.online) return;
  let sessionId = null, timer = null;
  const terminal = $('#terminal');
  async function loadJobs() {
    if (!sessionId) return;
    try {
      const { jobs } = await api(`/api/v1/sessions/${sessionId}/jobs`);
      terminal.innerHTML = jobsHTML(jobs);
      terminal.scrollTop = terminal.scrollHeight;
    } catch {}
  }
  $('#command-form').addEventListener('submit', async event => {
    event.preventDefault();
    const input = $('#command'), command = input.value.trim();
    if (!command) return;
    if (!sessionId) {
      sessionId = (await api(`/api/v1/devices/${deviceId}/sessions`, { method: 'POST', body: '{}' })).id;
      timer = setInterval(loadJobs, 1200);
    }
    await api(`/api/v1/sessions/${sessionId}/jobs`, { method: 'POST', body: JSON.stringify({ command }) });
    input.value = '';
    await loadJobs();
  });
  window.addEventListener('pagehide', () => { if (timer) clearInterval(timer); }, { once: true });
}
