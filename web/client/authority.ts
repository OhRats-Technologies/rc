import { api } from "./http";
import { syncWorkspaceAuthority } from "./control-client";
import { AuthoritySyncError } from "./authority-status";
import { freshPasskey, stepHeader } from "./step-up";

const workspaceId = document.querySelector<HTMLElement>("[data-authority-workspace]")?.dataset.authorityWorkspace || "";

document.querySelectorAll<HTMLButtonElement>("[data-authority-sync]").forEach(button => button.addEventListener("click", async () => {
  const id = button.dataset.authoritySync || "", output = document.querySelector<HTMLElement>(`[data-authority-status="${CSS.escape(id)}"]`);
  button.disabled = true; if (output) output.textContent = "SYNCING…";
  try {
    await syncWorkspaceAuthority(id);
    // The alert and status lines are rendered by the server, so reload to show the settled state.
    location.reload();
  } catch (error) {
    // A partial sync leaves the alert stale; refresh its pending list from the server's state.
    if (error instanceof AuthoritySyncError) {
      const pending = document.querySelector<HTMLElement>("[data-authority-pending]");
      if (pending) pending.textContent = error.state.deviceStates.filter(device => !device.synced).map(device => device.name).join(", ");
    }
    if (output) output.textContent = error instanceof Error ? error.message : String(error);
    button.disabled = false;
  }
}));

if (workspaceId) {
  document.querySelectorAll<HTMLFormElement>(`.role-form, form[action*="/workspaces/${CSS.escape(workspaceId)}/members/"][action$="/remove"]`).forEach(form => form.addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const parts = new URL(form.action).pathname.split("/"), memberId = parts[4];
      const step = await freshPasskey();
      if (form.classList.contains("role-form")) {
        const role = String(new FormData(form).get("role") || "");
        await api(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(memberId)}`,
          { method: "PATCH", headers: stepHeader(step), body: JSON.stringify({ role }) });
      } else {
        await api(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(memberId)}`,
          { method: "DELETE", headers: stepHeader(step) });
      }
      await syncWorkspaceAuthority(workspaceId); location.reload();
    } catch (error) {
      const target = form.closest<HTMLElement>(".access-row")?.querySelector<HTMLElement>(".row-error");
      if (target) target.textContent = error instanceof Error ? error.message : String(error);
    }
  }));
}
