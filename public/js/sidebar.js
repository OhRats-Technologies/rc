import { $, api, escapeHTML } from './api.js';

function workspaceIdFromPath() {
  return location.pathname.match(/^\/workspaces\/([^/]+)/)?.[1] || '';
}

function setActive() {
  const path = location.pathname;
  $('[data-nav="devices"]')?.classList.toggle('active', path.startsWith('/devices'));
  $('[data-nav="workspaces"]')?.classList.toggle('active', path.startsWith('/workspaces'));
  const workspaceId = workspaceIdFromPath();
  document.querySelectorAll('[data-workspace]').forEach(link => link.classList.toggle('active', link.dataset.workspace === workspaceId));
}

export function initializeSidebar(data) {
  const { user, workspaces } = data;
  $('#profile-name').textContent = user.name;
  $('#profile-initial').textContent = user.name.trim().slice(0, 1).toUpperCase() || '?';
  $('#workspace-links').innerHTML = workspaces.length ? workspaces.map(workspace => `
    <a class="workspace-link" data-workspace="${workspace.id}" href="/workspaces/${workspace.id}">
      <span class="workspace-dot"></span><span>${escapeHTML(workspace.name)}</span>
    </a>`).join('') : '<span class="meta">NO WORKSPACES</span>';
  setActive();

  const saved = localStorage.getItem('relay-sidebar');
  document.documentElement.dataset.sidebar = matchMedia('(max-width:760px)').matches ? 'closed' : (saved || 'open');
  $('#sidebar-toggle').addEventListener('click', () => {
    const closed = document.documentElement.dataset.sidebar === 'closed';
    document.documentElement.dataset.sidebar = closed ? 'open' : 'closed';
    localStorage.setItem('relay-sidebar', closed ? 'open' : 'closed');
    $('#sidebar-toggle').setAttribute('aria-expanded', String(closed));
    $('#sidebar-toggle').setAttribute('aria-label', closed ? 'Close sidebar' : 'Open sidebar');
  });
  $('#logout').addEventListener('click', async () => {
    await api('/api/v1/auth/logout', { method: 'POST', body: '{}' });
    location.href = '/';
  });
}
