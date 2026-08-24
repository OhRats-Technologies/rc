import { $ } from './api.js';
import { renderAccount } from './pages/account.js';
import { renderActivity } from './pages/activity.js';
import { renderDevice, renderDevices } from './pages/devices.js';
import { renderDeleteFleet, renderFleet, renderFleets, renderNewFleet } from './pages/fleets.js';
import { renderDeleteWorkspace, renderNewWorkspace, renderWorkspace, renderWorkspaces } from './pages/workspaces.js';

function notFound() {
  $('#page').innerHTML = '<section class="page"><div class="page-heading"><div><p class="eyebrow">404</p><h1>Not found</h1></div></div></section>';
}

export async function renderRoute() {
  const path = location.pathname;
  if (path === '/' || path === '/devices') return renderDevices();
  if (path === '/workspaces') return renderWorkspaces();
  if (path === '/workspaces/new') return renderNewWorkspace();
  if (path === '/account') return renderAccount();
  let match = path.match(/^\/devices\/([^/]+)$/);
  if (match) return renderDevice(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)$/);
  if (match) return renderWorkspace(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)\/activity$/);
  if (match) return renderActivity(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)\/delete$/);
  if (match) return renderDeleteWorkspace(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)\/fleets$/);
  if (match) return renderFleets(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)\/fleets\/new$/);
  if (match) return renderNewFleet(match[1]);
  match = path.match(/^\/workspaces\/([^/]+)\/fleets\/([^/]+)\/delete$/);
  if (match) return renderDeleteFleet(match[1], match[2]);
  match = path.match(/^\/workspaces\/([^/]+)\/fleets\/([^/]+)$/);
  if (match) return renderFleet(match[1], match[2]);
  notFound();
}
