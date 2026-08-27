import { api, copyText, qs } from "./http";
import { bytesToB64url, syncOwnedAuthorities } from "./control-client";
import { freshPasskey, stepHeader } from "./step-up";

type CreatedKey = { id: string; publicKey: string; scopes: string[]; expiresAt: number };

const dialog = qs<HTMLDialogElement>("[data-api-key-dialog]");
const form = qs<HTMLFormElement>("[data-api-key-form]");
const input = qs<HTMLInputElement>("[data-api-key-name]");
const error = qs<HTMLElement>("[data-api-key-error]");
const createView = qs<HTMLElement>("[data-api-key-create]");
const resultView = qs<HTMLElement>("[data-api-key-result]");
const secret = qs<HTMLElement>("[data-api-key-secret]");
const copy = qs<HTMLButtonElement>("[data-api-key-copy]");
const list = qs<HTMLElement>("#token-list");

function openCreate() {
  createView.hidden = false; resultView.hidden = true; error.textContent = ""; form.reset(); dialog.showModal();
  requestAnimationFrame(() => input.focus());
}

function close() { dialog.close(); }

function bindRevoke(revoke: HTMLFormElement) {
  revoke.addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const id = revoke.action.split("/").at(-2) || "";
      const step = await freshPasskey();
      await api(`/api/v1/tokens/${encodeURIComponent(id)}`, { method: "DELETE", headers: stepHeader(step) });
      await syncOwnedAuthorities(); revoke.closest(".token-row")?.remove();
      if (!list.querySelector(".token-row")) { const empty = document.createElement("p"); empty.className = "empty-state"; empty.textContent = "No API keys yet."; list.append(empty); }
    } catch (cause) { error.textContent = cause instanceof Error ? cause.message : String(cause); }
  });
}

function addKey(id: string, name: string, scopes: string[], expiresAt: number, lifetimeLabel: string) {
  list.querySelector(".empty-state")?.remove();
  const row = document.createElement("div"); row.className = "setting-row token-row";
  const main = document.createElement("div"); main.className = "token-row-main";
  const icon = document.createElement("span"); icon.className = "ui-icon icon-key"; icon.setAttribute("aria-hidden", "true");
  const copy = document.createElement("div"), title = document.createElement("strong"), meta = document.createElement("div");
  title.textContent = name; meta.className = "meta"; meta.textContent = `${scopes.join(" · ").toUpperCase()} · NEVER USED · ${expiresAt === 0 ? "UNTIL REVOKED" : `EXPIRES IN ${lifetimeLabel.toUpperCase()}`}`; copy.append(title, meta); main.append(icon, copy);
  const revoke = document.createElement("form"); revoke.method = "post"; revoke.action = `/api/tokens/${id}/delete`;
  const button = document.createElement("button"); button.className = "text-button"; button.type = "submit"; button.textContent = "REVOKE"; revoke.append(button);
  row.append(main, revoke); bindRevoke(revoke); list.prepend(row);
}

document.querySelector<HTMLElement>("[data-api-key-new]")?.addEventListener("click", event => { event.preventDefault(); openCreate(); });
document.querySelectorAll<HTMLButtonElement>("[data-api-key-cancel],[data-api-key-done]").forEach(button => button.addEventListener("click", close));
dialog.addEventListener("click", event => { if (event.target === dialog) close(); });
dialog.addEventListener("close", () => { createView.hidden = false; resultView.hidden = true; secret.textContent = ""; copy.dataset.copyValue = ""; });

form.addEventListener("submit", async event => {
  event.preventDefault(); const name = input.value.trim(); if (!name) { input.focus(); return; }
  const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; submit.disabled = true; error.textContent = "";
  try {
    const data = new FormData(form), scopes = Array.from(form.querySelectorAll<HTMLInputElement>('input[name="scope"]:checked')).map(item => item.value);
    const lifetime = String(data.get("lifetime") || "never");
    const lifetimeLabel = form.querySelector<HTMLSelectElement>('select[name="lifetime"]')?.selectedOptions[0]?.textContent || lifetime;
    const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
    const publicKey = bytesToB64url(await crypto.subtle.exportKey("raw", pair.publicKey));
    const privateKey = bytesToB64url(await crypto.subtle.exportKey("pkcs8", pair.privateKey));
    const step = await freshPasskey();
    const created = await api<CreatedKey>("/api/v1/tokens", { method: "POST", headers: stepHeader(step), body: JSON.stringify({ name, scopes, publicKey, lifetime }) });
    const apiSecret = `rcsk_${created.id}_${privateKey}`;
    addKey(created.id, name, created.scopes, created.expiresAt, lifetimeLabel); secret.textContent = apiSecret; copy.dataset.copyValue = apiSecret;
    await syncOwnedAuthorities();
    createView.hidden = true; resultView.hidden = false; requestAnimationFrame(() => copy.focus());
  } catch (cause) { error.textContent = cause instanceof Error ? cause.message : String(cause); }
  finally { submit.disabled = false; }
});

copy.addEventListener("click", () => void copyText(copy.dataset.copyValue || "", copy));

list.querySelectorAll<HTMLFormElement>('form[action^="/api/tokens/"][action$="/delete"]').forEach(bindRevoke);
