import { qs } from "./http";
import { createPasskey } from "./webauthn";

document.querySelector<HTMLButtonElement>("#add-passkey")?.addEventListener("click", async () => {
  try {
    await createPasskey("/api/v1/passkeys/options", "/api/v1/passkeys/verify", {});
    location.reload();
  } catch (error) { qs<HTMLElement>("#passkey-error").textContent = error instanceof Error ? error.message : String(error); }
});
