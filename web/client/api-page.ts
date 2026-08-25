import { api, copyText, qs } from "./http";

type CreatedKey = { id: string; token: string };

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

function addKey(id: string, name: string) {
  list.querySelector(".empty-state")?.remove();
  const row = document.createElement("div"); row.className = "setting-row token-row";
  const main = document.createElement("div"); main.className = "token-row-main";
  const icon = document.createElement("span"); icon.className = "ui-icon icon-key"; icon.setAttribute("aria-hidden", "true");
  const copy = document.createElement("div"), title = document.createElement("strong"), meta = document.createElement("div");
  title.textContent = name; meta.className = "meta"; meta.textContent = "NEVER USED"; copy.append(title, meta); main.append(icon, copy);
  const revoke = document.createElement("form"); revoke.method = "post"; revoke.action = `/api/tokens/${id}/delete`;
  const button = document.createElement("button"); button.className = "text-button"; button.type = "submit"; button.textContent = "REVOKE"; revoke.append(button);
  row.append(main, revoke); list.prepend(row);
}

document.querySelector<HTMLElement>("[data-api-key-new]")?.addEventListener("click", event => { event.preventDefault(); openCreate(); });
document.querySelectorAll<HTMLButtonElement>("[data-api-key-cancel],[data-api-key-done]").forEach(button => button.addEventListener("click", close));
dialog.addEventListener("click", event => { if (event.target === dialog) close(); });
dialog.addEventListener("close", () => { createView.hidden = false; resultView.hidden = true; secret.textContent = ""; copy.dataset.copyValue = ""; });

form.addEventListener("submit", async event => {
  event.preventDefault(); const name = input.value.trim(); if (!name) { input.focus(); return; }
  const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; submit.disabled = true; error.textContent = "";
  try {
    const created = await api<CreatedKey>("/api/v1/tokens", { method: "POST", body: JSON.stringify({ name }) });
    addKey(created.id, name); secret.textContent = created.token; copy.dataset.copyValue = created.token;
    createView.hidden = true; resultView.hidden = false; requestAnimationFrame(() => copy.focus());
  } catch (cause) { error.textContent = cause instanceof Error ? cause.message : String(cause); }
  finally { submit.disabled = false; }
});

copy.addEventListener("click", () => void copyText(copy.dataset.copyValue || "", copy));
