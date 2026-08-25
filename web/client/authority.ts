import { api } from "./http";
import { syncWorkspaceAuthority } from "./control-client";

const workspaceId = document.querySelector<HTMLElement>("[data-authority-workspace]")?.dataset.authorityWorkspace || "";

async function showStatus(id: string) {
  const output = document.querySelector<HTMLElement>(`[data-authority-status="${CSS.escape(id)}"]`); if (!output) return;
  try {
    const state = await api<{ devices: number; synced: number }>(`/api/v1/workspaces/${encodeURIComponent(id)}/authority`);
    output.textContent = !state.devices ? "NO NODES" : state.synced === state.devices ? `SYNCED ${state.synced}/${state.devices} NODES` : `PENDING RC LOCK SYNC · ${state.synced}/${state.devices} NODES`;
  } catch {}
}

document.querySelectorAll<HTMLButtonElement>("[data-authority-sync]").forEach(button => button.addEventListener("click", async () => {
  const workspaceId = button.dataset.authoritySync || "", output = document.querySelector<HTMLElement>(`[data-authority-status="${CSS.escape(workspaceId)}"]`);
  button.disabled = true; if (output) output.textContent = "SYNCING…";
  try {
    const result = await syncWorkspaceAuthority(workspaceId);
    if (output) output.textContent = result.devices ? `SYNCED ${result.synced}/${result.devices} NODES` : "NO NODES";
  } catch (error) { if (output) output.textContent = error instanceof Error ? error.message : String(error); }
  finally { button.disabled = false; }
}));

if (workspaceId) {
  void showStatus(workspaceId);
  document.querySelectorAll<HTMLFormElement>(`.role-form, form[action*="/workspaces/${CSS.escape(workspaceId)}/members/"][action$="/remove"]`).forEach(form => form.addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const parts = new URL(form.action).pathname.split("/"), memberId = parts[4];
      if (form.classList.contains("role-form")) {
        const role = String(new FormData(form).get("role") || "");
        await api(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(memberId)}`, { method: "PATCH", body: JSON.stringify({ role }) });
      } else {
        await api(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(memberId)}`, { method: "DELETE" });
      }
      await syncWorkspaceAuthority(workspaceId); location.reload();
    } catch (error) {
      const target = form.closest<HTMLElement>(".access-row")?.querySelector<HTMLElement>(".row-error");
      if (target) target.textContent = error instanceof Error ? error.message : String(error);
    }
  }));
}
