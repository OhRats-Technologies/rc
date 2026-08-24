import { api } from "./http";
import { onEvent } from "./socket";
import type { Device, RelayEvent, RemoteProcess } from "../types";

function setPresence(deviceId: string, online: boolean) {
  document.querySelectorAll<HTMLElement>(`[data-device-status="${CSS.escape(deviceId)}"]`).forEach(element => {
    element.classList.toggle("online", online); element.textContent = online ? "ONLINE" : "OFFLINE";
  });
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`);
  if (page) { const status = page.querySelector<HTMLElement>("#device-status"); if (status) { status.classList.toggle("online", online); status.textContent = online ? "ONLINE" : "OFFLINE"; } }
}

async function refreshDevice(deviceId: string) {
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`); if (!page) return;
  const { device } = await api<{ device: Device }>(`/api/v1/devices/${deviceId}`);
  page.querySelector<HTMLElement>("#node-agent")!.textContent = device.agent_version;
  page.querySelector<HTMLElement>("#node-capabilities")!.textContent = device.capabilities.map(v => v.toUpperCase()).join(" · ") || "NONE";
  const update = page.querySelector<HTMLButtonElement>("#update-node"); if (update) update.disabled = !device.online;
}

function activity(event: RelayEvent) {
  const workspaceId = document.querySelector<HTMLElement>("[data-activity-page]")?.dataset.activityPage;
  if (!event.audit || !workspaceId || event.workspaceId !== workspaceId) return;
  const list = document.querySelector<HTMLElement>("#activity-list"); if (!list) return;
  list.querySelector(".empty-state")?.remove(); const detail = event.detail || {};
  const value = String(detail.name || detail.command || detail.deviceId || detail.processId || "").slice(0, 120);
  const row = document.createElement("div"); row.className = "activity-row";
  row.innerHTML = `<span></span><span></span><time>NOW</time>`; row.children[0].textContent = event.kind.toUpperCase(); row.children[1].textContent = value;
  list.prepend(row); while (list.children.length > 100) list.lastElementChild?.remove();
}

async function refreshProcessList(deviceId: string) {
  const list = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"] #process-list`); if (!list) return;
  const { processes } = await api<{ processes: RemoteProcess[] }>(`/api/v1/devices/${deviceId}/processes`);
  for (const process of processes) {
    const state = list.querySelector<HTMLElement>(`[data-process-status="${CSS.escape(process.id)}"]`);
    if (state) { state.textContent = process.status === "running" ? "RUNNING" : process.status.toUpperCase(); state.classList.toggle("online", process.status === "running"); }
  }
}

onEvent(event => {
  if (event.kind === "device.online" && event.deviceId) setPresence(event.deviceId, true);
  if (event.kind === "device.offline" && event.deviceId) setPresence(event.deviceId, false);
  if (event.kind === "device.updated" && event.deviceId) void refreshDevice(event.deviceId);
  if (event.kind.startsWith("process.") && event.deviceId) void refreshProcessList(event.deviceId);
  if (event.kind === "node.update.ready" && event.deviceId) document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(event.deviceId)}"] #update-state`)!.textContent = "Restarting node…";
  if (event.kind === "node.update.error" && event.deviceId) document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(event.deviceId)}"] #update-state`)!.textContent = String(event.detail?.error || "Update failed.");
  activity(event);
});
