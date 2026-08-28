import { api } from "./http";
import { onEvent } from "./events";
import type { Device, RCEvent } from "../types";

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
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`);
  if (page) {
    const status = page.querySelector<HTMLElement>("#device-status");
    if (status) { status.classList.toggle("online", online); status.textContent = online ? "ONLINE" : "OFFLINE"; }
    const supportsProcess = page.dataset.supportsProcess === "true";
    const terminal = page.querySelector<HTMLButtonElement>("#open-terminal");
    if (terminal) terminal.disabled = !online || !supportsProcess;
  }
}

async function refreshDevice(deviceId: string) {
  const page = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"]`); if (!page) return;
  const { device } = await api<{ device: Device }>(`/api/v1/devices/${deviceId}`);
  page.querySelector<HTMLElement>("#node-version")!.textContent = device.version;
  const supportsProcess = device.capabilities.includes("process"); page.dataset.supportsProcess = String(supportsProcess);
  const terminal = page.querySelector<HTMLButtonElement>("#open-terminal");
  if (terminal) terminal.disabled = !device.online || !supportsProcess;
  const processError = page.querySelector<HTMLElement>("#process-error"); if (processError && supportsProcess) processError.textContent = "";
}


async function recoverLiveState() {
  await recoverEnrollment();
  try {
    const { devices } = await api<{ devices: Device[] }>("/api/v1/devices");
    for (const device of devices) setPresence(device.id, device.online);
    const page = document.querySelector<HTMLElement>("[data-device-page]");
    const current = page?.dataset.devicePage || "";
    if (current) {
      await refreshDevice(current);
      const { processes } = await api<{ processes: Array<{ id: string; status: string }> }>(`/api/v1/devices/${encodeURIComponent(current)}/processes`);
      const list = page?.querySelector<HTMLElement>("#process-list");
      if (list) for (const process of processes) {
        const state = list.querySelector<HTMLElement>(`[data-process-status="${CSS.escape(process.id)}"]`);
        if (!state) continue;
        state.textContent = process.status.toUpperCase(); state.classList.toggle("online", process.status === "running");
      }
    }
  } catch {}
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

function updateProcessList(event: RCEvent) {
  const deviceId = event.deviceId || "", processId = event.processId || "";
  if (!deviceId || !processId) return;
  const list = document.querySelector<HTMLElement>(`[data-device-page="${CSS.escape(deviceId)}"] #process-list`); if (!list) return;
  const state = list.querySelector<HTMLElement>(`[data-process-status="${CSS.escape(processId)}"]`); if (!state) return;
  const status = event.kind === "process.started" ? "RUNNING" : event.kind === "process.lost" ? "LOST" : event.kind === "process.exited" ? "EXITED" : "";
  if (!status) return;
  state.textContent = status; state.classList.toggle("online", status === "RUNNING");
  const page = list.closest<HTMLElement>("[data-device-page]");
  if (status !== "RUNNING" && page?.dataset.retainsProcessHistory !== "true") {
    state.closest<HTMLElement>("[data-process-row]")?.remove();
    if (!list.querySelector("[data-process-row]")) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = "No active processes.";
      list.append(empty);
    }
  }
}

onEvent(event => {
  if (event.kind === "rc.connected") { void recoverLiveState(); return; }
  if (event.kind === "device.enrolled" && event.workspaceId === enrollWorkspace && event.deviceId) finishEnrollment(event.deviceId);
  if (event.kind === "device.online" && event.deviceId) setPresence(event.deviceId, true);
  if (event.kind === "device.offline" && event.deviceId) setPresence(event.deviceId, false);
  if (event.kind === "device.updated" && event.deviceId) void refreshDevice(event.deviceId);
  if (event.kind.startsWith("process.")) updateProcessList(event);
  activity(event);
});

void recoverLiveState();
