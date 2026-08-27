import { api, copyText } from "./http";

document.querySelectorAll<HTMLFormElement>("[data-json-form]").forEach(form => form.addEventListener("submit", async event => {
  event.preventDefault();
  const path = form.dataset.path || "", redirect = form.dataset.redirect || location.pathname;
  const data = Object.fromEntries(new FormData(form).entries());
  const button = form.querySelector<HTMLButtonElement>('button[type="submit"],button:not([type])');
  if (button) button.disabled = true;
  try {
    await api(path, { method: form.dataset.method || "POST", body: JSON.stringify(data) });
    location.assign(redirect);
  } catch (error) {
    let output = form.querySelector<HTMLElement>(".error");
    if (!output) { output = document.createElement("p"); output.className = "error"; form.append(output); }
    output.textContent = error instanceof Error ? error.message : String(error);
    if (button) button.disabled = false;
  }
}));

document.querySelectorAll<HTMLFormElement>("[data-enrollment-form]").forEach(form => form.addEventListener("submit", async event => {
  event.preventDefault();
  const workspaceId = String(new FormData(form).get("workspaceId") || "");
  const output = document.querySelector<HTMLElement>("[data-enrollment-result]");
  const field = document.querySelector<HTMLElement>("[data-enrollment-copy-field]");
  const error = document.querySelector<HTMLElement>("[data-enrollment-error]");
  const button = form.querySelector<HTMLButtonElement>("button"); if (button) button.disabled = true;
  if (error) error.textContent = "";
  try {
    const result = await api<{ install: string; expiresAt: number }>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/enrollments`, { method: "POST", body: "{}" });
    if (output) output.textContent = result.install;
    if (field) field.hidden = false;
  } catch (error) {
    if (field) field.hidden = true;
    const message = error instanceof Error ? error.message : String(error);
    const errorOutput = document.querySelector<HTMLElement>("[data-enrollment-error]"); if (errorOutput) errorOutput.textContent = message;
  } finally { if (button) button.disabled = false; }
}));

document.querySelectorAll<HTMLButtonElement>("[data-enrollment-copy]").forEach(button => button.addEventListener("click", () => {
  const value = document.querySelector<HTMLElement>("[data-enrollment-result]")?.textContent || "";
  if (value) void copyText(value, button);
}));

document.querySelectorAll<HTMLFormElement>('form[action$="/invites"]').forEach(form => form.addEventListener("submit", async event => {
  event.preventDefault();
  const parts = new URL(form.action).pathname.split("/"), workspaceId = parts[2] || "";
  const role = String(new FormData(form).get("role") || "operator");
  const button = form.querySelector<HTMLButtonElement>("button"); if (button) button.disabled = true;
  try {
    const result = await api<{ url: string; expiresAt: number }>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/invites`, {
      method: "POST", body: JSON.stringify({ role }),
    });
    const host = document.querySelector<HTMLElement>("[data-invite-result]");
    if (host) {
      host.replaceChildren();
      const label = document.createElement("span"); label.className = "meta"; label.textContent = `${role.toUpperCase()} INVITE · SHOWN ONCE`;
      const field = document.createElement("div"); field.className = "or-copy-field invite-link";
      const code = document.createElement("code"); code.textContent = result.url;
      const copy = document.createElement("button"); copy.type = "button"; copy.className = "or-copy-button"; copy.setAttribute("aria-label", "Copy invite link");
      copy.innerHTML = '<span class="or-copy-icon" aria-hidden="true"></span>';
      copy.addEventListener("click", () => void copyText(result.url, copy));
      field.append(code, copy); host.append(label, field);
    }
  } catch (error) {
    let output = form.querySelector<HTMLElement>(".error");
    if (!output) { output = document.createElement("p"); output.className = "error"; form.append(output); }
    output.textContent = error instanceof Error ? error.message : String(error);
  } finally { if (button) button.disabled = false; }
}));

document.querySelectorAll<HTMLFormElement>('form[action$="/revoke"]').forEach(form => form.addEventListener("submit", async event => {
  event.preventDefault();
  const parts = new URL(form.action).pathname.split("/");
  const workspaceId = parts[2] || "", inviteId = parts[4] || "";
  try {
    await api(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/invites/${encodeURIComponent(inviteId)}`, { method: "DELETE" });
    form.closest(".setting-row")?.remove();
  } catch (error) {
    const output = document.querySelector<HTMLElement>("[data-invite-result]");
    if (output) output.textContent = error instanceof Error ? error.message : String(error);
  }
}));
