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
      <div class="job-command"><span class="prompt">$</span><span>${escapeHTML(job.payload.command)}</span><span class="job-state">${job.status === 'sent' ? (job.started_at ? 'RUNNING' : 'QUEUED') : escapeHTML(job.status.toUpperCase())}</span></div>
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
  const [{ device }, { session }] = await Promise.all([
    api(`/api/v1/devices/${deviceId}`), api(`/api/v1/devices/${deviceId}/sessions`),
  ]);
  let sessionId = session?.id || null, syncTimer = null, historyIndex = 0;
  const history = [];
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading">
        <div><p class="eyebrow">DEVICE</p><h1>${escapeHTML(device.name)}</h1><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)} · ${escapeHTML(device.hostname)}</div></div>
        <span id="device-status" class="status ${device.online ? 'online' : ''}">${device.online ? 'ONLINE' : `LAST SEEN ${relative(device.last_seen)}`}</span>
      </div>
      <div class="device-console-grid">
        <section class="console-panel">
          <div class="console-bar">
            <span>RELAY://NODE/${escapeHTML(device.name.toUpperCase())}/SHELL</span>
            <div class="console-actions"><span id="session-state">${sessionId ? `SESSION ${sessionId.slice(0, 8)}` : 'NO SESSION'}</span><button id="new-session" class="text-button" type="button">NEW SESSION</button></div>
          </div>
          <div id="terminal" class="terminal"><span class="muted">${sessionId ? 'Loading session…' : device.online ? 'Ready.' : 'Offline.'}</span></div>
          <form id="command-form" class="command-row">
            <span class="command-prompt">$</span>
            <input id="command" autocomplete="off" spellcheck="false" placeholder="command" ${device.online ? '' : 'disabled'} required>
            <button class="primary-button" type="submit" ${device.online ? '' : 'disabled'}>EXEC</button>
          </form>
        </section>
        <aside class="node-inspector">
          <section><p class="eyebrow">NODE</p><dl class="fact-list">
            <div><dt>HOST</dt><dd id="node-host">${escapeHTML(device.hostname)}</dd></div>
            <div><dt>OS</dt><dd id="node-os">${escapeHTML(device.platform.toUpperCase())}</dd></div>
            <div><dt>ARCH</dt><dd id="node-arch">${escapeHTML(device.arch)}</dd></div>
            <div><dt>AGENT</dt><dd id="node-agent">${escapeHTML(device.agent_version)}</dd></div>
          </dl></section>
          <section><p class="eyebrow">FLEETS</p><div class="inspector-lines">${device.fleets.map(fleet => `<span>${escapeHTML(fleet.workspace_name)} / ${escapeHTML(fleet.name)}</span>`).join('') || '<span>NONE</span>'}</div></section>
          <section><p class="eyebrow">CAPABILITIES</p><div class="inspector-lines">${device.capabilities.map(capability => `<span>${escapeHTML(capability.toUpperCase())}</span>`).join('') || '<span>NONE</span>'}</div></section>
        </aside>
      </div>
    </section>`;
  const terminal = $('#terminal');
  async function loadJobs() {
    if (!sessionId) return;
    try {
      const { jobs } = await api(`/api/v1/sessions/${sessionId}/jobs`);
      history.splice(0, history.length, ...jobs.map(job => job.payload.command));
      historyIndex = history.length;
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
    history.push(command); historyIndex = history.length;
    input.value = '';
    await loadJobs();
  });
  $('#command').addEventListener('keydown', event => {
    if (!history.length || !['ArrowUp','ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    historyIndex = event.key === 'ArrowUp' ? Math.max(0, historyIndex - 1) : Math.min(history.length, historyIndex + 1);
    event.currentTarget.value = historyIndex === history.length ? '' : history[historyIndex];
  });
  $('#new-session').addEventListener('click', async () => {
    sessionId = (await api(`/api/v1/devices/${deviceId}/sessions`, {
      method: 'POST', body: JSON.stringify({ fresh: true }),
    })).id;
    history.splice(0); historyIndex = 0;
    $('#session-state').textContent = `SESSION ${sessionId.slice(0, 8)}`;
    terminal.innerHTML = `<span class="muted">${$('#command').disabled ? 'Offline.' : 'Ready.'}</span>`;
  });
  if (sessionId) await loadJobs();
  onRelayEvent(event => {
    if (event.deviceId !== deviceId) return;
    if (event.kind.startsWith('job.') && event.sessionId === sessionId) {
      const pinned = terminal.scrollHeight - terminal.scrollTop - terminal.clientHeight < 80;
      if (!applyJobEvent(event)) scheduleSync();
      if (pinned) terminal.scrollTop = terminal.scrollHeight;
      return;
    }
    if (event.kind === 'device.online' || event.kind === 'device.offline' || event.kind === 'device.updated') {
      api(`/api/v1/devices/${deviceId}`).then(({ device: current }) => {
        const status = $('#device-status');
        status.classList.toggle('online', current.online);
        status.textContent = current.online ? 'ONLINE' : `LAST SEEN ${relative(current.last_seen)}`;
        $('#node-host').textContent = current.hostname;
        $('#node-os').textContent = current.platform.toUpperCase();
        $('#node-arch').textContent = current.arch;
        $('#node-agent').textContent = current.agent_version;
        $('#command').disabled = !current.online;
        $('#command-form button').disabled = !current.online;
        if (!sessionId && !terminal.querySelector('.job')) {
          terminal.innerHTML = `<span class="muted">${current.online ? 'Ready.' : 'Offline.'}</span>`;
        }
      }).catch(() => {});
    }
  });
}
