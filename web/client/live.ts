import { api } from "./http";
import { onEvent } from "./socket";
import type { Device, RCEvent, RemoteProcess } from "../types";

const enrollPage = document.querySelector<HTMLElement>("[data-enroll-page]");
const enrollWorkspace = enrollPage?.dataset.enrollPage || "";
const knownEnrollmentDevices = new Set((enrollPage?.dataset.knownDevices || "").split(",").filter(Boolean));

function finishEnrollment(deviceId: string) {
  if (!enrollPage || !deviceId) return;
  const state = document.querySelector<HTMLElement>("#enrollment-state"); if (state) state.textContent = "Device connected. Opening…";
  window.setTimeout(() => { location.href = `/devices/${deviceId}`; }, 350);
}

async function recoverEnrollment() {
  if (!enrollWorkspace) return;
  try {
    const { devices } = await api<{ devices: Device[] }>("/api/v1/devices");
    const added = devices.find(device => device.workspace_id === enrollWorkspace && !knownEnrollmentDevices.has(device.id));
    if (added) finishEnrollment(added.id);
  } catch {}
}

function setPresence(deviceId: string, online: boolean) {
  document.querySelectorAll<HTMLElement>(`[data-device-status="${CSS.escape(deviceId)}"]`).forEach(element => {
    element.classList.toggle("online", online); element.textContent = online ? "ONLINE" : "OFFLINE";
  });
  document.querySelectorAll<HTMLElement>(`[data-sidebar-device-status="${CSS.escape(deviceId)}"]`).forEach(element => element.classList.toggle("online", online));
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`);
  if (page) {
    const status = page.querySelector<HTMLElement>("#device-status");
    if (status) { status.classList.toggle("online", online); status.textContent = online ? "ONLINE" : "OFFLINE"; }
    const supportsProcess = page.dataset.supportsProcess === "true";
    const terminal = page.querySelector<HTMLButtonElement>("#open-terminal"), start = page.querySelector<HTMLButtonElement>("#process-launch button[type=submit]");
    if (terminal) terminal.disabled = !online || !supportsProcess;
    if (start) start.disabled = !online || !supportsProcess;
    const update = page.querySelector<HTMLButtonElement>("#update-node"); if (update) update.disabled = !online;
    if (online) void refreshDevice(deviceId);
  }
}

async function refreshDevice(deviceId: string) {
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`); if (!page) return;
  const { device } = await api<{ device: Device }>(`/api/v1/devices/${deviceId}`);
  page.querySelector<HTMLElement>("#node-agent")!.textContent = device.agent_version;
  const supportsProcess = device.capabilities.includes("process"); page.dataset.supportsProcess = String(supportsProcess);
  const terminal = page.querySelector<HTMLButtonElement>("#open-terminal"), start = page.querySelector<HTMLButtonElement>("#process-launch button[type=submit]");
  if (terminal) terminal.disabled = !device.online || !supportsProcess;
  if (start) start.disabled = !device.online || !supportsProcess;
  const processError = page.querySelector<HTMLElement>("#process-error"); if (processError && supportsProcess) processError.textContent = "";
  const update = page.querySelector<HTMLButtonElement>("#update-node"), updateState = page.querySelector<HTMLElement>("#update-state");
  if (update) update.disabled = !device.online;
  if (update && updateState && device.agent_version === page.dataset.rcVersion) {
    update.hidden = true;
    updateState.textContent = `Updated to ${device.agent_version}.`;
  }
}

function activity(event: RCEvent) {
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
  if (event.kind === "device.enrolled" && event.workspaceId === enrollWorkspace && event.deviceId) finishEnrollment(event.deviceId);
  if (event.kind === "device.online" && event.deviceId) setPresence(event.deviceId, true);
  if (event.kind === "device.offline" && event.deviceId) setPresence(event.deviceId, false);
  if (event.kind === "device.updated" && event.deviceId) void refreshDevice(event.deviceId);
  if (event.kind.startsWith("process.") && event.deviceId) void refreshProcessList(event.deviceId);
  if (event.kind === "node.update.ready" && event.deviceId) document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(event.deviceId)}"] #update-state`)!.textContent = "Restarting node…";
  if (event.kind === "node.update.complete" && event.deviceId) {
    const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(event.deviceId)}"]`);
    const state = page?.querySelector<HTMLElement>("#update-state"), button = page?.querySelector<HTMLButtonElement>("#update-node");
    if (state) state.textContent = `Updated to ${String(event.detail?.version || "current version")}.`;
    if (button) button.hidden = true;
  }
  if (event.kind === "node.update.error" && event.deviceId) document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(event.deviceId)}"] #update-state`)!.textContent = String(event.detail?.error || "Update failed.");
  activity(event);
});

void recoverEnrollment();
