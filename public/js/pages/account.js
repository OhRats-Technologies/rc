import { $, api, copyText, escapeHTML, relative } from '../api.js';
import { createPasskey } from '../webauthn.js';

async function loadAccount() {
  const [{ user }, { passkeys }, { tokens }] = await Promise.all([
    api('/api/v1/me'), api('/api/v1/passkeys'), api('/api/v1/tokens'),
  ]);
  return { user, passkeys, tokens };
}

function passkeyRows(passkeys) {
  return passkeys.length ? passkeys.map((passkey, index) => `
    <div class="list-row">
      <div><strong>Passkey ${index + 1}</strong><div class="meta">${passkey.last_used ? `USED ${relative(passkey.last_used)}` : 'NOT USED YET'}</div></div>
      <button class="icon-button" data-delete-passkey="${passkey.id}" type="button" aria-label="Delete passkey"><span class="ui-icon icon-trash"></span></button>
    </div>`).join('') : '<div class="list-row"><span class="muted">No passkeys. This session is your last access.</span></div>';
}

function tokenRows(tokens) {
  return tokens.length ? tokens.map(token => `
    <div class="list-row">
      <div><strong>${escapeHTML(token.name)}</strong><div class="meta">${token.last_used ? `USED ${relative(token.last_used)}` : 'NEVER USED'}</div></div>
      <button class="icon-button" data-delete-token="${token.id}" type="button" aria-label="Delete API token"><span class="ui-icon icon-trash"></span></button>
    </div>`).join('') : '<div class="list-row"><span class="muted">No API tokens.</span></div>';
}

export async function renderAccount() {
  const { user, passkeys, tokens } = await loadAccount();
  $('#page').innerHTML = `
    <section class="page">
      <div class="page-heading"><div><p class="eyebrow">ACCOUNT</p><h1>${escapeHTML(user.name)}</h1></div></div>
      <section class="panel">
        <div class="page-heading"><div><p class="eyebrow">PASSKEYS</p><p class="muted">Passkeys are the only way to sign in.</p></div><button id="add-passkey" class="primary-button secondary-button" type="button"><span class="ui-icon icon-add"></span>ADD PASSKEY</button></div>
        <div id="passkey-list" class="list">${passkeyRows(passkeys)}</div>
      </section>
      <section class="panel">
        <p class="eyebrow">API TOKENS</p>
        <form id="token-form" class="form-stack">
          <label>Name<input name="name" placeholder="Automation" required></label>
          <button class="primary-button secondary-button" type="submit">CREATE API TOKEN</button>
        </form>
        <div id="token-result" class="result" hidden></div>
        <div id="token-list" class="list result">${tokenRows(tokens)}</div>
      </section>
    </section>`;

  $('#add-passkey').addEventListener('click', async () => {
    await createPasskey('/api/v1/passkeys/options', '/api/v1/passkeys/verify', {});
    location.reload();
  });
  $('#passkey-list').addEventListener('click', async event => {
    const button = event.target.closest('[data-delete-passkey]'); if (!button) return;
    await api(`/api/v1/passkeys/${button.dataset.deletePasskey}`, { method: 'DELETE', body: '{}' });
    location.reload();
  });
  $('#token-form').addEventListener('submit', async event => {
    event.preventDefault();
    const name = new FormData(event.currentTarget).get('name');
    const out = await api('/api/v1/tokens', { method: 'POST', body: JSON.stringify({ name }) });
    const result = $('#token-result'); result.hidden = false;
    result.innerHTML = `<p class="muted">Shown once.</p><pre>${escapeHTML(out.token)}</pre><button id="copy-token" class="text-button" type="button">COPY TOKEN</button>`;
    $('#copy-token').addEventListener('click', click => copyText(out.token, click.currentTarget));
  });
  $('#token-list').addEventListener('click', async event => {
    const button = event.target.closest('[data-delete-token]'); if (!button) return;
    await api(`/api/v1/tokens/${button.dataset.deleteToken}`, { method: 'DELETE', body: '{}' });
    location.reload();
  });
}
