import { api, qs } from "./http";
import { request } from "./socket";

const page = document.querySelector<HTMLElement>("[data-device-page]");
const deviceId = page?.dataset.devicePage || "";

async function start(command: string, cwd = "") {
  const result = await request<{ processId: string }>({ type: "process.allocate", deviceId, cols: 80, rows: 24 });
  sessionStorage.setItem(`rc_process_start_${result.processId}`, JSON.stringify({ command, cwd, cols: 80, rows: 24 }));
  location.href = `/devices/${deviceId}/processes/${result.processId}`;
}

document.querySelector<HTMLButtonElement>("#open-terminal")?.addEventListener("click", async () => {
  try { await start('exec "${SHELL:-sh}" -l'); }
  catch (error) { qs<HTMLElement>("#process-error").textContent = error instanceof Error ? error.message : String(error); }
});

const deviceName = document.querySelector<HTMLElement>("[data-device-title-view]");
const deviceNameForm = document.querySelector<HTMLFormElement>("[data-device-title-form]");
const deviceNameInput = deviceNameForm?.querySelector<HTMLInputElement>('input[name="name"]');
const deviceRename = document.querySelector<HTMLButtonElement>("[data-device-title-rename]");
const deviceRenameError = document.querySelector<HTMLElement>("[data-device-title-error]");
let deviceSubmitting = false;

function cancelDeviceRename() {
  if (!deviceName || !deviceNameForm || !deviceNameInput || !deviceRename) return;
  deviceName.hidden = false; deviceNameForm.hidden = true; deviceRename.hidden = false;
  deviceNameInput.value = deviceNameInput.defaultValue;
}

deviceRename?.addEventListener("click", () => {
  if (!deviceName || !deviceNameForm || !deviceNameInput || !deviceRename) return;
  deviceName.hidden = true; deviceNameForm.hidden = false; deviceRename.hidden = true;
  deviceNameInput.focus(); deviceNameInput.select();
});
deviceNameForm?.addEventListener("submit", async event => {
  event.preventDefault();
  if (!deviceName || !deviceNameForm || !deviceNameInput || !deviceRename || !deviceId) return;
  const name = deviceNameInput.value.trim();
  if (!name) return;
  deviceSubmitting = true;
  try {
    const result = await api<{ name: string }>(`/api/v1/devices/${encodeURIComponent(deviceId)}`, { method: "PATCH", body: JSON.stringify({ name }) });
    deviceName.textContent = result.name; deviceNameInput.value = result.name; deviceNameInput.defaultValue = result.name;
    document.title = `${result.name} | RC`;
    const sidebarName = document.querySelector<HTMLElement>(`[data-sidebar-device="${CSS.escape(deviceId)}"] .workspace-device-name > span`); if (sidebarName) sidebarName.textContent = result.name;
    const sidebarInput = document.querySelector<HTMLInputElement>(`[data-sidebar-device="${CSS.escape(deviceId)}"] .device-rename-input`); if (sidebarInput) { sidebarInput.value = result.name; sidebarInput.defaultValue = result.name; }
    document.querySelectorAll<HTMLElement>(`[data-delete-kind="device"][data-delete-endpoint="/api/v1/devices/${CSS.escape(deviceId)}"]`).forEach(button => button.dataset.deleteName = result.name);
    if (deviceRenameError) deviceRenameError.textContent = "";
    deviceName.hidden = false; deviceNameForm.hidden = true; deviceRename.hidden = false;
  } catch (error) { if (deviceRenameError) deviceRenameError.textContent = error instanceof Error ? error.message : String(error); }
  finally { deviceSubmitting = false; }
});
deviceNameForm?.addEventListener("keydown", event => {
  if (event.key === "Escape") { event.preventDefault(); cancelDeviceRename(); }
  if (event.key === "Enter") { event.preventDefault(); deviceNameForm.requestSubmit(); }
});
deviceNameInput?.addEventListener("blur", () => window.setTimeout(() => {
  if (!deviceSubmitting && document.activeElement !== deviceNameInput) cancelDeviceRename();
}));
