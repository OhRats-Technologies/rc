import { qs } from "./http";
import { createPasskey } from "./webauthn";
import { api } from "./http";
import { syncOwnedAuthorities } from "./control-client";
import { freshPasskey, stepHeader } from "./step-up";

document.querySelector<HTMLButtonElement>("#add-passkey")?.addEventListener("click", async () => {
  try {
    const step = await freshPasskey();
    await createPasskey("/api/v1/passkeys/options", "/api/v1/passkeys/verify", {}, stepHeader(step));
    await syncOwnedAuthorities();
    location.reload();
  } catch (error) { qs<HTMLElement>("#passkey-error").textContent = error instanceof Error ? error.message : String(error); }
});

document.querySelectorAll<HTMLFormElement>('form[action^="/account/passkeys/"][action$="/delete"]').forEach(form => form.addEventListener("submit", async event => {
  event.preventDefault();
  try {
    const passkeyId = form.action.split("/").at(-2) || "";
    const step = await freshPasskey();
    await api(`/api/v1/passkeys/${encodeURIComponent(passkeyId)}`, { method: "DELETE", headers: stepHeader(step) });
    await syncOwnedAuthorities(); location.reload();
  } catch (error) { qs<HTMLElement>("#passkey-error").textContent = error instanceof Error ? error.message : String(error); }
}));

const accountName = document.querySelector<HTMLElement>("[data-account-name-view]");
const accountNameForm = document.querySelector<HTMLFormElement>("[data-account-name-form]");
const accountNameInput = accountNameForm?.querySelector<HTMLInputElement>('input[name="name"]');
const accountRename = document.querySelector<HTMLButtonElement>("[data-account-rename]");
const accountError = document.querySelector<HTMLElement>(".account-title-error");
let accountSubmitting = false;

function cancelAccountRename() {
  if (!accountName || !accountNameForm || !accountNameInput || !accountRename) return;
  accountName.hidden = false; accountNameForm.hidden = true; accountRename.hidden = false;
  accountNameInput.value = accountNameInput.defaultValue;
}

accountRename?.addEventListener("click", () => {
  if (!accountName || !accountNameForm || !accountNameInput) return;
  accountName.hidden = true; accountNameForm.hidden = false; accountRename.hidden = true;
  accountNameInput.focus(); accountNameInput.select();
});
accountNameForm?.addEventListener("submit", async event => {
  event.preventDefault();
  if (!accountName || !accountNameForm || !accountNameInput || !accountRename) return;
  const name = accountNameInput.value.trim();
  if (!name) { if (accountError) accountError.textContent = "Account name required."; return; }
  accountSubmitting = true;
  try {
    const response = await fetch("/account/name", {
      method: "POST", headers: { accept: "application/json", "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ name }),
    });
    const result = await response.json() as { name?: string; error?: string };
    if (!response.ok || !result.name) throw new Error(result.error || "Rename failed.");
    accountName.textContent = result.name; accountNameInput.value = result.name; accountNameInput.defaultValue = result.name;
    document.querySelector<HTMLElement>(".profile-name")!.textContent = result.name;
    const initial = document.querySelector<HTMLElement>(".profile-initial"); if (initial) initial.textContent = result.name.trim().slice(0, 1).toUpperCase() || "?";
    if (accountError) accountError.textContent = "";
    accountName.hidden = false; accountNameForm.hidden = true; accountRename.hidden = false;
  } catch (error) { if (accountError) accountError.textContent = error instanceof Error ? error.message : String(error); }
  finally { accountSubmitting = false; }
});
accountNameForm?.addEventListener("keydown", event => {
  if (event.key === "Escape") { event.preventDefault(); cancelAccountRename(); }
  if (event.key === "Enter") { event.preventDefault(); accountNameForm.requestSubmit(); }
});
accountNameInput?.addEventListener("blur", () => window.setTimeout(() => {
  if (!accountSubmitting && document.activeElement !== accountNameInput) cancelAccountRename();
}));
