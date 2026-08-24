const $ = (s) => document.querySelector(s);
const authShell = $('#auth');
const dashboardEl = $('#dashboard');
const setupForm = $('#setup-form');
const loginForm = $('#login-form');
const registerForm = $('#register-form');
const errorEl = $('#auth-error');
const dialog = $('#token-dialog');

let state = { data: null, workspaceId: null, selectedDevice: null, sessionId: null, jobs: [], refresh: null, jobRefresh: null };

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { 'content-type': 'application/json', ...(options.headers || {}) }
  });
  let data = {};
  try { data = await response.json(); } catch {}
  if (!response.ok) throw Object.assign(new Error(data.error || response.statusText), { status: response.status });
  return data;
}

function payload(form) { return Object.fromEntries(new FormData(form).entries()); }
function escapeHTML(v = '') { return String(v).replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c])); }
function relative(ts) {
  if (!ts) return 'NEVER';
  const seconds = Math.max(0, Math.round((Date.now() - ts) / 1000));
  if (seconds < 60) return `${seconds}S AGO`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M AGO`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H AGO`;
  return `${Math.floor(seconds / 86400)}D AGO`;
}

function showAuth(mode, invite = '') {
  dashboardEl.hidden = true; authShell.hidden = false; $('#logout').hidden = true; $('#connection-state').hidden = true;
  setupForm.hidden = mode !== 'setup'; loginForm.hidden = mode !== 'login'; registerForm.hidden = mode !== 'register';
  $('#auth-title').textContent = mode === 'setup' ? 'Create Relay' : mode === 'register' ? 'Join Relay' : 'Sign in';
  $('#auth-copy').textContent = mode === 'setup' ? 'Create the first account and personal workspace.' : mode === 'register' ? 'Create an account with your workspace invite.' : 'Connect to your devices.';
  if (mode === 'register') registerForm.elements.invite.value = invite;
}

async function boot() {
  const status = await api('/api/v1/status');
  if (status.setupRequired) return showAuth('setup');
  const invite = new URLSearchParams(location.search).get('invite');
  try {
    await loadDashboard();
    if (invite) {
      await api('/api/v1/workspaces/join', { method: 'POST', body: JSON.stringify({ token: invite }) });
      history.replaceState({}, '', '/');
      await loadDashboard();
    }
  } catch (err) {
    if (err.status === 401) {
      return showAuth(invite ? 'register' : 'login', invite || '');
    }
    throw err;
  }
}

for (const [form, path] of [[setupForm, '/api/v1/auth/setup'], [loginForm, '/api/v1/auth/login'], [registerForm, '/api/v1/auth/register']]) {
  form.addEventListener('submit', async (event) => {
    event.preventDefault(); errorEl.textContent = '';
    try { await api(path, { method: 'POST', body: JSON.stringify(payload(form)) }); history.replaceState({}, '', '/'); await loadDashboard(); }
    catch (err) { errorEl.textContent = err.message; }
  });
}

$('#logout').addEventListener('click', async () => { await api('/api/v1/auth/logout', { method: 'POST', body: '{}' }); clearTimers(); showAuth('login'); });

function clearTimers() {
  clearInterval(state.refresh); clearInterval(state.jobRefresh); state.refresh = null; state.jobRefresh = null;
}

async function loadDashboard(workspaceId = state.workspaceId) {
  const suffix = workspaceId ? `?workspace=${encodeURIComponent(workspaceId)}` : '';
  const data = await api(`/api/v1/dashboard${suffix}`);
  state.data = data; state.workspaceId = data.workspace?.id || null;
  authShell.hidden = true; dashboardEl.hidden = false; $('#logout').hidden = false; $('#connection-state').hidden = false;
  renderDashboard();
  if (!state.refresh) state.refresh = setInterval(() => loadDashboard(state.workspaceId).catch(() => {}), 5000);
}

function renderDashboard() {
  const data = state.data;
  const workspaceSelect = $('#workspace-select');
  workspaceSelect.innerHTML = data.workspaces.map(w => `<option value="${w.id}" ${w.id === data.workspace?.id ? 'selected' : ''}>${escapeHTML(w.name)}</option>`).join('');
  const role = data.workspace?.role;
  $('#invite-member').hidden = role !== 'owner';
  $('#new-fleet').hidden = !['owner','member'].includes(role);

  $('#fleets').innerHTML = data.fleets.length ? data.fleets.map(f => `
    <div class="fleet-row">
      <span class="fleet-name">${escapeHTML(f.name)}</span>
      ${['owner','member'].includes(role) ? `<button class="text-button enroll-button" data-enroll="${f.id}">ENROLL DEVICE</button>` : ''}
    </div>`).join('') : `<div class="fleet-row muted">No fleets.</div>`;

  const online = data.devices.filter(d => d.online).length;
  $('#device-count').textContent = `${data.devices.length} ${data.devices.length === 1 ? 'device' : 'devices'}`;
  $('#online-count').textContent = `${online} ONLINE`;
  $('#devices').innerHTML = data.devices.length ? data.devices.map(d => `
    <button class="device-row ${state.selectedDevice?.id === d.id ? 'selected' : ''}" data-device="${d.id}">
      <span class="device-row-top"><span class="device-name">${escapeHTML(d.name)}</span><span class="status-dot ${d.online ? 'online' : ''}" title="${d.online ? 'Online' : 'Offline'}"></span></span>
      <span class="device-meta">${escapeHTML(d.platform.toUpperCase())}/${escapeHTML(d.arch)} · ${escapeHTML(d.fleets || '')}</span>
      <span class="device-meta">${d.online ? 'ONLINE' : `LAST SEEN ${relative(d.last_seen)}`}</span>
    </button>`).join('') : `<div class="device-row muted">No devices yet. Enroll one from a fleet.</div>`;

  $('#events').innerHTML = data.events.length ? data.events.slice(0, 12).map(e => `
    <div class="event"><span>${escapeHTML(e.kind.toUpperCase())}</span><span class="event-detail">${escapeHTML(eventDetail(e))}</span><span class="event-time">${relative(e.created_at)}</span></div>`).join('') : `<span class="muted mono">NO ACTIVITY</span>`;

  if (state.selectedDevice) {
    const fresh = data.devices.find(d => d.id === state.selectedDevice.id);
    if (fresh) { state.selectedDevice = fresh; renderConsoleHeader(); }
  }
}

function eventDetail(e) {
  if (e.detail?.name) return e.detail.name;
  if (e.detail?.command) return e.detail.command;
  if (e.device_id) return e.device_id.slice(0, 8);
  return '';
}

$('#workspace-select').addEventListener('change', e => { state.selectedDevice = null; state.sessionId = null; stopJobPolling(); loadDashboard(e.target.value); });

$('#new-workspace').addEventListener('click', async () => {
  const name = await ask('New workspace', 'Workspace name'); if (!name) return;
  const out = await api('/api/v1/workspaces', { method: 'POST', body: JSON.stringify({ name }) });
  await loadDashboard(out.id);
});

$('#new-fleet').addEventListener('click', async () => {
  if (!state.workspaceId) return;
  const name = await ask('New fleet', 'Fleet name'); if (!name) return;
  await api(`/api/v1/workspaces/${state.workspaceId}/fleets`, { method: 'POST', body: JSON.stringify({ name }) });
  await loadDashboard();
});

$('#invite-member').addEventListener('click', async () => {
  const out = await api(`/api/v1/workspaces/${state.workspaceId}/invites`, { method: 'POST', body: JSON.stringify({ role: 'member' }) });
  showToken('Workspace invite', 'Share this link once. It expires in 24 hours.', `${location.origin}/?invite=${encodeURIComponent(out.token)}`);
});

$('#api-token').addEventListener('click', async () => {
  const out = await api('/api/v1/tokens', { method: 'POST', body: JSON.stringify({ name: 'Dashboard token' }) });
  showToken('API token', 'Shown once. Use as Authorization: Bearer TOKEN.', out.token);
});

$('#fleets').addEventListener('click', async e => {
  const button = e.target.closest('[data-enroll]'); if (!button) return;
  const out = await api(`/api/v1/fleets/${button.dataset.enroll}/enrollments`, { method: 'POST', body: '{}' });
  showToken('Enroll device', 'Run this on the device. The token is single-use and expires in 24 hours.', out.install);
});

$('#devices').addEventListener('click', async e => {
  const button = e.target.closest('[data-device]'); if (!button) return;
  state.selectedDevice = state.data.devices.find(d => d.id === button.dataset.device);
  state.sessionId = null; state.jobs = []; stopJobPolling();
  renderDashboard(); renderConsoleHeader(); renderJobs();
});

function renderConsoleHeader() {
  const d = state.selectedDevice; if (!d) return;
  $('#empty-console').hidden = true; $('#active-console').hidden = false;
  $('#console-device').textContent = d.name;
  $('#console-status').textContent = d.online ? 'ONLINE' : 'OFFLINE';
  $('#command').disabled = !d.online;
  $('#command-form button').disabled = !d.online;
}

$('#command-form').addEventListener('submit', async e => {
  e.preventDefault(); if (!state.selectedDevice?.online) return;
  const input = $('#command'); const command = input.value.trim(); if (!command) return;
  if (!state.sessionId) {
    const session = await api(`/api/v1/devices/${state.selectedDevice.id}/sessions`, { method: 'POST', body: '{}' });
    state.sessionId = session.id; startJobPolling();
  }
  await api(`/api/v1/sessions/${state.sessionId}/jobs`, { method: 'POST', body: JSON.stringify({ command }) });
  input.value = ''; await loadJobs();
});

function startJobPolling() { stopJobPolling(); state.jobRefresh = setInterval(loadJobs, 1200); }
function stopJobPolling() { clearInterval(state.jobRefresh); state.jobRefresh = null; }
async function loadJobs() {
  if (!state.sessionId) return;
  try { state.jobs = (await api(`/api/v1/sessions/${state.sessionId}/jobs`)).jobs; renderJobs(); } catch {}
}
function renderJobs() {
  const terminal = $('#terminal');
  terminal.innerHTML = state.jobs.length ? state.jobs.map(j => `
    <div class="job ${j.status === 'failed' ? 'job-failed' : ''}">
      <div class="job-command">$ ${escapeHTML(j.payload.command)}</div>
      <div class="job-result">${j.result == null ? `[${escapeHTML(j.status)}]` : escapeHTML(j.result)}${j.exit_code != null && j.exit_code !== 0 ? `\n[exit ${j.exit_code}]` : ''}</div>
    </div>`).join('') : `<span class="muted">Session ready.</span>`;
  terminal.scrollTop = terminal.scrollHeight;
}

function showToken(title, copy, value) {
  $('#dialog-title').textContent = title; $('#dialog-copy').textContent = copy; $('#dialog-value').textContent = value; dialog.showModal();
}
$('#copy-dialog').addEventListener('click', async () => { await navigator.clipboard.writeText($('#dialog-value').textContent); $('#copy-dialog').textContent = 'COPIED'; setTimeout(() => $('#copy-dialog').textContent = 'COPY', 1000); });

function ask(title, label) {
  const d = $('#input-dialog');
  const form = $('#input-dialog-form');
  const input = $('#input-dialog-value');
  $('#input-dialog-title').textContent = title;
  $('#input-dialog-label').textContent = label;
  input.value = '';
  d.showModal();
  setTimeout(() => input.focus(), 0);
  return new Promise(resolve => {
    let settled = false;
    const finish = value => {
      if (settled) return;
      settled = true;
      form.removeEventListener('submit', submit);
      d.removeEventListener('close', close);
      resolve(value);
    };
    const submit = event => {
      event.preventDefault();
      const value = input.value.trim();
      if (!value) return;
      d.close();
      finish(value);
    };
    const close = () => finish(null);
    form.addEventListener('submit', submit);
    d.addEventListener('close', close);
  });
}

boot().catch(err => { console.error(err); showAuth('login'); errorEl.textContent = err.message; });

