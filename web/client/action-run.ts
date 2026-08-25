import { api } from "./http";
import { openControlSession } from "./control-session";

const page = document.querySelector<HTMLElement>("[data-action-run]");
const actionId = page?.dataset.actionRun || "", command = page?.dataset.actionCommand || "", cwd = page?.dataset.actionCwd || "";

document.querySelector<HTMLFormElement>(`.action-run-form, form[action="/actions/${CSS.escape(actionId)}/run"]`)?.addEventListener("submit", async event => {
  event.preventDefault(); const form = event.currentTarget as HTMLFormElement, data = new FormData(form);
  const deviceIds = data.getAll("deviceId").map(String).filter(Boolean); if (!deviceIds.length) return;
  const button = form.querySelector<HTMLButtonElement>('button[type="submit"]'); if (button) button.disabled = true;
  try {
    const result = await api<{ results: Array<{ deviceId: string; deviceName: string; processId?: string; error?: string }> }>(`/api/v1/actions/${encodeURIComponent(actionId)}/run`, {
      method: "POST", body: JSON.stringify({ deviceIds }),
    });
    const started = result.results.filter(item => item.processId && !item.error);
    if (started.length === 1) {
      const item = started[0];
      sessionStorage.setItem(`rc_process_start_${item.processId}`, JSON.stringify({ command, cwd, cols: 80, rows: 24 }));
      location.href = `/devices/${item.deviceId}/processes/${item.processId}`; return;
    }
    await Promise.all(started.map(async item => {
      const control = await openControlSession(item.deviceId);
      try { await control.send({ type: "process.start", id: item.processId, command, cwd, cols: 80, rows: 24 }); }
      finally { window.setTimeout(() => control.close(), 300); }
    }));
    location.reload();
  } catch (error) {
    const target = page?.querySelector<HTMLElement>(".action-results") || page;
    if (target) { const message = document.createElement("p"); message.className = "error"; message.textContent = error instanceof Error ? error.message : String(error); target.append(message); }
    if (button) button.disabled = false;
  }
});
