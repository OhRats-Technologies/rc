import { api, qs } from "./http";
import { passkeyAssertion } from "./webauthn";

const shell = qs<HTMLElement>("[data-cli-client]"), clientId = shell.dataset.cliClient || "", signingPublicKey = shell.dataset.cliPublicKey || "";
const form = qs<HTMLFormElement>('form[action="/cli/login"]');

form.addEventListener("submit", async event => {
  event.preventDefault(); const button = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const code = String(new FormData(form).get("code") || ""), error = qs<HTMLElement>(".error");
  try {
    const start = await api<{ authorizationId: string; options: any }>("/api/v1/control/authorize/options", {
      method: "POST", body: JSON.stringify({ clientId, signingPublicKey }),
    });
    await api("/api/v1/control/authorize/verify", { method: "POST", body: JSON.stringify({ authorizationId: start.authorizationId, response: await passkeyAssertion(start.options) }) });
    await api("/api/v1/auth/cli/approve", { method: "POST", body: JSON.stringify({ code }) });
    location.reload();
  } catch (cause) { error.textContent = cause instanceof Error ? cause.message : String(cause); button.disabled = false; }
});
