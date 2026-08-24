import { ApiError, api, formObject, qs } from "./api";
import { authenticatePasskey, createPasskey } from "./webauthn";

type AuthMode = "setup" | "login" | "register";

function errorMessage(error: unknown) {
  if (error instanceof DOMException && error.name === "NotAllowedError") return "Passkey request cancelled.";
  return error instanceof Error ? error.message : "Authentication failed.";
}

export function showAuth(mode: AuthMode, invite = "") {
  qs<HTMLElement>("#site-shell").hidden = true;
  qs<HTMLElement>("#auth").hidden = false;
  qs<HTMLFormElement>("#setup-form").hidden = mode !== "setup";
  qs<HTMLElement>("#login-options").hidden = mode !== "login";
  qs<HTMLFormElement>("#register-form").hidden = mode !== "register";
  qs<HTMLElement>("#auth-title").textContent = mode === "setup" ? "Create Relay" : mode === "register" ? "Join Relay" : "Sign in";
  qs<HTMLElement>("#auth-copy").textContent = mode === "setup" ? "Create the first account with a passkey."
    : mode === "register" ? "Create a passkey to join this workspace." : "Use a passkey or a full-access API token.";
  if (mode === "register") qs<HTMLInputElement>('#register-form [name="invite"]').value = invite;
}

export function bindAuth() {
  qs<HTMLFormElement>("#setup-form").addEventListener("submit", async event => {
    event.preventDefault(); qs<HTMLElement>("#auth-error").textContent = "";
    const form = event.currentTarget as HTMLFormElement;
    try {
      await createPasskey("/api/v1/auth/setup/options", "/api/v1/auth/setup/verify", formObject(form));
      location.href = "/devices";
    } catch (error) { qs<HTMLElement>("#auth-error").textContent = errorMessage(error); }
  });
  qs<HTMLFormElement>("#login-form").addEventListener("submit", async event => {
    event.preventDefault(); qs<HTMLElement>("#auth-error").textContent = "";
    try {
      await authenticatePasskey();
      const invite = new URLSearchParams(location.search).get("invite");
      if (invite) await api("/api/v1/workspaces/join", { method: "POST", body: JSON.stringify({ token: invite }) });
      location.href = "/devices";
    } catch (error) { qs<HTMLElement>("#auth-error").textContent = errorMessage(error); }
  });
  qs<HTMLFormElement>("#token-login-form").addEventListener("submit", async event => {
    event.preventDefault(); qs<HTMLElement>("#auth-error").textContent = "";
    const form = event.currentTarget as HTMLFormElement;
    try {
      const token = String(new FormData(form).get("token") || "");
      await api("/api/v1/auth/token", { method: "POST", body: JSON.stringify({ token }) });
      const invite = new URLSearchParams(location.search).get("invite");
      if (invite) await api("/api/v1/workspaces/join", { method: "POST", body: JSON.stringify({ token: invite }) });
      location.href = "/devices";
    } catch (error) { qs<HTMLElement>("#auth-error").textContent = errorMessage(error); }
  });
  qs<HTMLFormElement>("#register-form").addEventListener("submit", async event => {
    event.preventDefault(); qs<HTMLElement>("#auth-error").textContent = "";
    const form = event.currentTarget as HTMLFormElement;
    try {
      await createPasskey("/api/v1/auth/register/options", "/api/v1/auth/register/verify", formObject(form));
      location.href = "/devices";
    } catch (error) { qs<HTMLElement>("#auth-error").textContent = errorMessage(error); }
  });
  qs<HTMLButtonElement>("#existing-account").addEventListener("click", () => showAuth("login"));
}

export function isUnauthenticated(error: unknown) { return error instanceof ApiError && error.status === 401; }
