import { qs } from "./http";
import { authenticatePasskey, createPasskey } from "./webauthn";

function message(error: unknown) {
  if (error instanceof DOMException && error.name === "NotAllowedError") return "Passkey request cancelled.";
  return error instanceof Error ? error.message : "Authentication failed.";
}
function errorOut(error: unknown) { qs<HTMLElement>("#auth-error").textContent = message(error); }
function destination() {
  const next = document.querySelector<HTMLElement>(".auth-content")?.dataset.authNext;
  return next || (location.search.includes("invite=") ? location.href : `${location.origin}/devices`);
}

document.querySelector<HTMLFormElement>("#setup-form")?.addEventListener("submit", async event => {
  event.preventDefault(); const form = event.currentTarget as HTMLFormElement;
  try {
    const name = String(new FormData(form).get("name") || "");
    await createPasskey("/api/v1/auth/setup/options", "/api/v1/auth/setup/verify", { name }); location.href = "/devices";
  } catch (error) { errorOut(error); }
});
document.querySelector<HTMLFormElement>("#login-form")?.addEventListener("submit", async event => {
  event.preventDefault(); try { await authenticatePasskey(); location.href = destination(); } catch (error) { errorOut(error); }
});
document.querySelector<HTMLFormElement>("#register-form")?.addEventListener("submit", async event => {
  event.preventDefault(); const form = event.currentTarget as HTMLFormElement, data = new FormData(form);
  try {
    await createPasskey("/api/v1/auth/register/options", "/api/v1/auth/register/verify", { name: data.get("name"), invite: data.get("invite") });
    location.href = "/devices";
  } catch (error) { errorOut(error); }
});
