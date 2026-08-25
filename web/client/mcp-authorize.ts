import { api, qs } from "./http";
import { ensureControlAuthorized, signControl, syncWorkspaceAuthority } from "./control-client";

const root = qs<HTMLElement>("[data-mcp-request]"), form = qs<HTMLFormElement>("[data-mcp-form]"), error = qs<HTMLElement>("[data-mcp-error]");

form.addEventListener("submit", async event => {
  event.preventDefault(); error.textContent = "";
  const button = form.querySelector<HTMLButtonElement>("button[type=submit]"); if (button) button.disabled = true;
  try {
    const data = new FormData(form), deviceIds = data.getAll("device").map(String), scopes = data.getAll("scope").map(String);
    if (!deviceIds.length) throw new Error("Select at least one machine.");
    if (!scopes.length) throw new Error("Select at least one permission.");
    const prepared = await api<{ grant: string; signaturePayload: string }>("/oauth/authorize/prepare", {
      method: "POST", body: JSON.stringify({ requestId: root.dataset.mcpRequest, deviceIds, scopes }),
    });
    await ensureControlAuthorized(true);
    const signed = await signControl(prepared.signaturePayload);
    const result = await api<{ redirect: string; grantId: string; workspaceIds: string[]; requiresSync: boolean }>("/oauth/authorize/approve", {
      method: "POST", body: JSON.stringify({ requestId: root.dataset.mcpRequest, controlClientId: signed.clientId, signature: signed.signature }),
    });
    if (result.requiresSync) try { for (const workspaceId of result.workspaceIds) await syncWorkspaceAuthority(workspaceId); }
    catch (cause) {
      const revoked = await api<{ workspaceIds: string[] }>(`/oauth/grants/${encodeURIComponent(result.grantId)}`, { method: "DELETE" });
      try { for (const workspaceId of revoked.workspaceIds) await syncWorkspaceAuthority(workspaceId); } catch {}
      throw cause;
    }
    location.assign(result.redirect);
  } catch (cause) { error.textContent = cause instanceof Error ? cause.message : String(cause); if (button) button.disabled = false; }
});
