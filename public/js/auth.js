import { $, api, formJSON } from './api.js';
import { authenticatePasskey, createPasskey } from './webauthn.js';

const authShell = $('#auth');
const setupForm = $('#setup-form');
const loginForm = $('#login-form');
const registerForm = $('#register-form');
const errorEl = $('#auth-error');

export function showAuth(mode, invite = '') {
  $('#site-shell').hidden = true;
  authShell.hidden = false;
  setupForm.hidden = mode !== 'setup';
  loginForm.hidden = mode !== 'login';
  registerForm.hidden = mode !== 'register';
  $('#auth-title').textContent = mode === 'setup' ? 'Create Relay' : mode === 'register' ? 'Join Relay' : 'Sign in';
  $('#auth-copy').textContent = mode === 'setup' ? 'Create the first account with a passkey.'
    : mode === 'register' ? 'Create a passkey to join this workspace.' : 'Use a passkey to continue.';
  if (mode === 'register') registerForm.elements.invite.value = invite;
}

function errorMessage(error) {
  if (error?.name === 'NotAllowedError') return 'Passkey request cancelled.';
  return error?.message || 'Authentication failed.';
}

export function bindAuth() {
  setupForm.addEventListener('submit', async event => {
    event.preventDefault(); errorEl.textContent = '';
    try {
      await createPasskey('/api/v1/auth/setup/options', '/api/v1/auth/setup/verify', formJSON(setupForm));
      location.href = '/devices';
    } catch (error) { errorEl.textContent = errorMessage(error); }
  });
  loginForm.addEventListener('submit', async event => {
    event.preventDefault(); errorEl.textContent = '';
    try {
      await authenticatePasskey();
      const invite = new URLSearchParams(location.search).get('invite');
      if (invite) await api('/api/v1/workspaces/join', { method: 'POST', body: JSON.stringify({ token: invite }) });
      location.href = '/devices';
    } catch (error) { errorEl.textContent = errorMessage(error); }
  });
  registerForm.addEventListener('submit', async event => {
    event.preventDefault(); errorEl.textContent = '';
    try {
      await createPasskey('/api/v1/auth/register/options', '/api/v1/auth/register/verify', formJSON(registerForm));
      location.href = '/devices';
    } catch (error) { errorEl.textContent = errorMessage(error); }
  });
  $('#existing-account').addEventListener('click', () => showAuth('login'));
}
