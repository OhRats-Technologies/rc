import { $, api, escapeHTML, relative } from '../api.js';
import { onRelayEvent } from '../events.js';

function deviceRows(devices) {
  return devices.length ? devices.map(device => `
    <a class="list-row" href="/devices/${device.id}">
      <div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.workspaces || '')}</div></div>
      <span class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `SEEN ${relative(device.last_seen)}`}</span>
    </a>`).join('') : '<div class="list-row"><span class="muted">No devices yet. Enroll one from a fleet.</span></div>';
}

export async function renderDevices() {
  const { devices } = await api('/api/v1/devices');
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">DEVICES</p><h1>Devices</h1></div></div>
      <div id="device-list" class="list">${deviceRows(devices)}</div>
    </section>`;
  onRelayEvent(event => {
    if (!event.kind.startsWith('device.') && event.kind !== 'fleet.device_added') return;
    api('/api/v1/devices').then(data => { $('#device-list').innerHTML = deviceRows(data.devices); }).catch(() => {});
  });
}

function jobsHTML(jobs) {
  return jobs.length ? jobs.map(job => `
    <div class="job" data-job="${job.id}" data-exit="${job.exit_code ?? ''}">
      <div class="job-command"><span class="prompt">›</span>${escapeHTML(job.payload.command)}<span class="job-state">${job.status === 'sent' ? (job.started_at ? 'RUNNING' : 'QUEUED') : escapeHTML(job.status.toUpperCase())}</span></div>
      <div class="job-result">${job.result == null ? '' : escapeHTML(job.result)}${job.exit_code != null && job.exit_code !== 0 ? `\n[exit ${job.exit_code}]` : ''}</div>
    </div>`).join('') : '<span class="muted">Session ready.</span>';
}

function jobElement(jobId) { return document.querySelector(`[data-job="${CSS.escape(jobId)}"]`); }

function applyJobEvent(event) {
  const job = jobElement(event.jobId);
  if (!job) return false;
  const state = job.querySelector('.job-state'), result = job.querySelector('.job-result');
  if (event.kind === 'job.started') state.textContent = 'RUNNING';
  if (event.kind === 'job.output' && event.detail?.chunk) {
    result.textContent += event.detail.chunk;
  }
  if (event.kind === 'job.finished') {
    state.textContent = String(event.detail?.status || 'finished').toUpperCase();
    if (event.detail?.message && !result.textContent.endsWith(event.detail.message)) result.textContent += event.detail.message;
    if (Number.isInteger(event.detail?.exitCode) && event.detail.exitCode !== 0 && job.dataset.exit !== String(event.detail.exitCode)) {
      result.textContent += `\n[exit ${event.detail.exitCode}]`;
      job.dataset.exit = String(event.detail.exitCode);
    }
  }
  return true;
}

export async function renderDevice(deviceId) {
  const { device } = await api(`/api/v1/devices/${deviceId}`);
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading">
        <div><p class="eyebrow">DEVICE</p><h1>${escapeHTML(device.name)}</h1><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.hostname)}</div></div>
        <span id="device-status" class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `LAST SEEN ${relative(device.last_seen)}`}</span>
      </div>
      <section class="panel">
        <p class="eyebrow">FLEETS</p>
        <div class="meta">${device.fleets.map(fleet => `${escapeHTML(fleet.workspace_name)} / ${escapeHTML(fleet.name)}`).join(' · ') || 'NONE'}</div>
      </section>
      <section class="panel">
        <div class="console-bar"><span>COMMAND CONSOLE</span><span id="session-state">IDLE</span></div>
        <div id="terminal" class="terminal"><span class="muted">${device.online ? 'Ready.' : 'Offline.'}</span></div>
        <form id="command-form" class="command-row">
          <input id="command" autocomplete="off" spellcheck="false" placeholder="uname -a" ${device.online ? '' : 'disabled'} required>
          <button class="primary-button" type="submit" ${device.online ? '' : 'disabled'}>RUN</button>
        </form>
      </section>
    </section>`;
  let sessionId = null, syncTimer = null;
  const terminal = $('#terminal');
  async function loadJobs() {
    if (!sessionId) return;
    try {
      const { jobs } = await api(`/api/v1/sessions/${sessionId}/jobs`);
      terminal.innerHTML = jobsHTML(jobs);
      terminal.scrollTop = terminal.scrollHeight;
    } catch {}
  }
  function scheduleSync() {
    if (syncTimer) return;
    syncTimer = setTimeout(() => { syncTimer = null; loadJobs(); }, 80);
  }
  $('#command-form').addEventListener('submit', async event => {
    event.preventDefault();
    const input = $('#command'), command = input.value.trim();
    if (!command) return;
    if (!sessionId) {
      sessionId = (await api(`/api/v1/devices/${deviceId}/sessions`, { method: 'POST', body: '{}' })).id;
      $('#session-state').textContent = `SESSION ${sessionId.slice(0, 8)}`;
    }
    await api(`/api/v1/sessions/${sessionId}/jobs`, { method: 'POST', body: JSON.stringify({ command }) });
    input.value = '';
    await loadJobs();
  });
  onRelayEvent(event => {
    if (event.deviceId !== deviceId) return;
    if (event.kind.startsWith('job.') && event.sessionId === sessionId) {
      if (!applyJobEvent(event)) scheduleSync();
      terminal.scrollTop = terminal.scrollHeight;
      return;
    }
    if (event.kind === 'device.online' || event.kind === 'device.offline') {
      api(`/api/v1/devices/${deviceId}`).then(({ device: current }) => {
        const status = $('#device-status');
        status.classList.toggle('online', current.online);
        status.textContent = current.online ? 'ONLINE' : `LAST SEEN ${relative(current.last_seen)}`;
        $('#command').disabled = !current.online;
        $('#command-form button').disabled = !current.online;
        if (!sessionId && !terminal.querySelector('.job')) {
          terminal.innerHTML = `<span class="muted">${current.online ? 'Ready.' : 'Offline.'}</span>`;
        }
      }).catch(() => {});
    }
  });
  addEventListener('pagehide', () => {
    if (!sessionId) return;
    fetch(`/api/v1/sessions/${sessionId}`, {
      method: 'DELETE', headers: { 'content-type': 'application/json' }, body: '{}', keepalive: true,
    }).catch(() => {});
  }, { once: true });
}
