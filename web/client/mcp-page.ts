import { api } from "./http";
import { syncWorkspaceAuthority } from "./control-client";

const error = document.querySelector<HTMLElement>("[data-mcp-page-error]");
document.querySelectorAll<HTMLButtonElement>("[data-mcp-revoke]").forEach(button => button.addEventListener("click", async () => {
  button.disabled = true; if (error) error.textContent = "";
  try {
    const revoked = await api<{ workspaceIds: string[] }>(`/oauth/grants/${encodeURIComponent(button.dataset.mcpRevoke || "")}`, { method: "DELETE" });
    for (const workspaceId of revoked.workspaceIds) await syncWorkspaceAuthority(workspaceId);
    location.reload();
  } catch (cause) {
    if (error) error.textContent = cause instanceof Error ? cause.message : String(cause);
    button.disabled = false;
  }
}));
