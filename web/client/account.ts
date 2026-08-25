import { qs } from "./http";
import { createPasskey } from "./webauthn";

document.querySelector<HTMLButtonElement>("#add-passkey")?.addEventListener("click", async () => {
  try {
    await createPasskey("/api/v1/passkeys/options", "/api/v1/passkeys/verify", {});
    location.reload();
  } catch (error) { qs<HTMLElement>("#passkey-error").textContent = error instanceof Error ? error.message : String(error); }
});

const accountName = document.querySelector<HTMLElement>("[data-account-name-view]");
const accountNameForm = document.querySelector<HTMLFormElement>("[data-account-name-form]");
const accountNameInput = accountNameForm?.querySelector<HTMLInputElement>('input[name="name"]');
const accountRename = document.querySelector<HTMLButtonElement>("[data-account-rename]");

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
accountNameForm?.addEventListener("keydown", event => {
  if (event.key === "Escape") { event.preventDefault(); cancelAccountRename(); }
});
accountNameInput?.addEventListener("blur", () => window.setTimeout(() => {
  if (document.activeElement !== accountNameInput) cancelAccountRename();
}));
