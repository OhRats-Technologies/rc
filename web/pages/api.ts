import { api, copyText, escapeHTML, qs, relative } from "../api";
import type { ApiToken } from "../types";

function tokenRows(tokens: ApiToken[]) {
  return tokens.length ? tokens.map(token => `<div class="setting-row">
    <div><strong>${escapeHTML(token.name)}</strong><div class="meta">${token.last_used ? `USED ${relative(token.last_used)}` : "NEVER USED"}</div></div>
    <button class="text-button" data-delete-token="${token.id}" type="button">REVOKE</button>
  </div>`).join("") : '<p class="empty-state">No API tokens.</p>';
}

export async function renderApi() {
  const { tokens } = await api<{ tokens: ApiToken[] }>("/api/v1/tokens");
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">API</p><h1>API access</h1><p class="page-copy">Tokens have full account access and can also sign into the web app.</p></div></header>
    <section class="content-section">
      <div class="section-heading"><div><p class="eyebrow">NEW TOKEN</p><h2>Create credential</h2></div></div>
      <form id="token-form" class="inline-form"><label>Name<input name="name" placeholder="Automation" required></label><button class="primary-button" type="submit">CREATE TOKEN</button></form>
      <div id="token-result" class="credential-result" hidden></div>
    </section>
    <section class="content-section">
      <div class="section-heading"><div><p class="eyebrow">TOKENS</p><h2>Active credentials</h2></div></div>
      <div id="token-list" class="settings-list">${tokenRows(tokens)}</div>
    </section>
    <section class="content-section api-note"><p class="eyebrow">AUTHORIZATION</p><code>Authorization: Bearer rly_…</code></section>
  </div>`;
  const refresh = async () => {
    const data = await api<{ tokens: ApiToken[] }>("/api/v1/tokens");
    qs<HTMLElement>("#token-list").innerHTML = tokenRows(data.tokens);
  };
  qs<HTMLFormElement>("#token-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const name = String(new FormData(form).get("name") || "");
    const out = await api<{ token: string }>("/api/v1/tokens", { method: "POST", body: JSON.stringify({ name }) });
    const result = qs<HTMLElement>("#token-result");
    result.hidden = false;
    result.innerHTML = `<p class="meta">SHOWN ONCE</p><code>${escapeHTML(out.token)}</code><button id="copy-token" class="text-button" type="button">COPY</button>`;
    qs<HTMLButtonElement>("#copy-token").addEventListener("click", event => copyText(out.token, event.currentTarget as HTMLButtonElement));
    form.reset();
    await refresh();
  });
  qs<HTMLElement>("#token-list").addEventListener("click", async event => {
    const button = (event.target as Element).closest<HTMLButtonElement>("[data-delete-token]");
    if (!button) return;
    await api(`/api/v1/tokens/${button.dataset.deleteToken}`, { method: "DELETE", body: "{}" });
    await refresh();
  });
}
