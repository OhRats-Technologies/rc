import { api, escapeHTML, qs, relative } from "../api";
import { createPasskey } from "../webauthn";
import type { Passkey, User } from "../types";

function passkeyRows(passkeys: Passkey[]) {
  return passkeys.length ? passkeys.map((passkey, index) => `<div class="setting-row">
    <div><strong>Passkey ${index + 1}</strong><div class="meta">${passkey.last_used ? `USED ${relative(passkey.last_used)}` : "NOT USED YET"}</div></div>
    <button class="text-button" data-delete-passkey="${passkey.id}" type="button">REMOVE</button>
  </div>`).join("") : '<p class="empty-state">No passkeys. This browser session is your remaining access.</p>';
}

async function load() {
  const [{ user }, { passkeys }] = await Promise.all([
    api<{ user: User }>("/api/v1/me"), api<{ passkeys: Passkey[] }>("/api/v1/passkeys"),
  ]);
  return { user, passkeys };
}

export async function renderAccount() {
  const { user, passkeys } = await load();
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">ACCOUNT</p><h1>${escapeHTML(user.name)}</h1></div></header>
    <section class="content-section">
      <div class="section-heading"><div><p class="eyebrow">PASSKEYS</p><h2>Sign-in credentials</h2></div><button id="add-passkey" class="text-button" type="button">ADD PASSKEY</button></div>
      <div id="passkey-list" class="settings-list">${passkeyRows(passkeys)}</div>
    </section>
  </div>`;
  const refresh = async () => {
    const { passkeys } = await api<{ passkeys: Passkey[] }>("/api/v1/passkeys");
    qs<HTMLElement>("#passkey-list").innerHTML = passkeyRows(passkeys);
  };
  qs<HTMLButtonElement>("#add-passkey").addEventListener("click", async () => {
    await createPasskey("/api/v1/passkeys/options", "/api/v1/passkeys/verify", {});
    await refresh();
  });
  qs<HTMLElement>("#passkey-list").addEventListener("click", async event => {
    const button = (event.target as Element).closest<HTMLButtonElement>("[data-delete-passkey]");
    if (!button) return;
    await api(`/api/v1/passkeys/${button.dataset.deletePasskey}`, { method: "DELETE", body: "{}" });
    await refresh();
  });
}
