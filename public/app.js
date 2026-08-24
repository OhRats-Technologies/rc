import { $, api } from './js/api.js';
import { bindAuth, showAuth } from './js/auth.js';
import { renderRoute } from './js/router.js';
import { initializeSidebar } from './js/sidebar.js';

bindAuth();

async function boot() {
  const status = await api('/api/v1/status');
  if (status.setupRequired) {
    showAuth('setup');
    if (!status.setupAuthorized) $('#auth-error').textContent = 'Open the Relay setup link first.';
    return;
  }
  const invite = new URLSearchParams(location.search).get('invite');
  let me;
  try { me = await api('/api/v1/me'); }
  catch (error) {
    if (error.status === 401) { showAuth(invite ? 'register' : 'login', invite || ''); return; }
    throw error;
  }
  if (invite) {
    await api('/api/v1/workspaces/join', { method: 'POST', body: JSON.stringify({ token: invite }) });
    location.href = '/devices';
    return;
  }
  $('#auth').hidden = true;
  $('#site-shell').hidden = false;
  initializeSidebar(me);
  await renderRoute();
}

boot().catch(error => {
  console.error(error);
  const page = $('#page');
  if (page) page.innerHTML = `<section class="page"><div class="panel"><p class="error">${String(error.message || error)}</p></div></section>`;
});
