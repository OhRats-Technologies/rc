import { api, copyText, escapeHTML, qs, relative } from "../api";
import { onRelayEvent, relayRequest } from "../events";
import { navigate } from "../navigation";
import type { Device, RemoteProcess, Status } from "../types";

function processState(process: RemoteProcess) {
  if (process.status === "starting") return "STARTING";
  if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST";
  return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}

function deviceRows(devices: Device[]) {
  return devices.length ? devices.map(device => `<a class="data-row" href="/devices/${device.id}">
    <div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.workspace_name)} · ${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div>
    <span class="status ${device.online ? "online" : ""}">${device.online ? "ONLINE" : `SEEN ${relative(device.last_seen)}`}</span>
  </a>`).join("") : '<p class="empty-state">No devices yet. Enroll one from a workspace.</p>';
}

function processRows(deviceId: string, processes: RemoteProcess[]) {
  return processes.length ? processes.map(process => `<a class="data-row process-row" href="/devices/${deviceId}/processes/${process.id}">
    <div><strong class="mono">${escapeHTML(process.command)}</strong><div class="meta">${escapeHTML(process.cwd || "~")} · ${relative(process.created_at)}</div></div>
    <span class="status ${process.status === "running" ? "online" : ""}">${escapeHTML(processState(process))}</span>
  </a>`).join("") : '<p class="empty-state">No processes yet.</p>';
}

export async function renderDevices() {
  const { devices } = await api<{ devices: Device[] }>("/api/v1/devices");
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">DEVICES</p><h1>Devices</h1></div></header>
    <div id="device-list" class="data-list">${deviceRows(devices)}</div>
  </div>`;
  const refresh = async () => {
    const data = await api<{ devices: Device[] }>("/api/v1/devices");
    qs<HTMLElement>("#device-list").innerHTML = deviceRows(data.devices);
  };
  return onRelayEvent(event => {
    if (event.kind === "relay.connected" || event.kind.startsWith("device.")) void refresh();
  });
}

export async function renderDevice(deviceId: string) {
  const [{ device }, { processes }, relay] = await Promise.all([
    api<{ device: Device }>(`/api/v1/devices/${deviceId}`),
    api<{ processes: RemoteProcess[] }>(`/api/v1/devices/${deviceId}/processes`),
    api<Status>("/api/v1/status"),
  ]);
  const supportsProcess = device.capabilities.includes("process");
  const supportsUpdate = device.capabilities.includes("update");
  const updateCommand = "curl -fsSL https://relay.ohrats.party/install.sh | sh";
  let updateInFlight = false;

  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header device-header">
      <div><p class="eyebrow">DEVICE</p><h1>${escapeHTML(device.name)}</h1><p class="meta"><a href="/workspaces/${device.workspace_id}">${escapeHTML(device.workspace_name)}</a> · ${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</p></div>
      <span id="device-status" class="status ${device.online ? "online" : ""}">${device.online ? "ONLINE" : `LAST SEEN ${relative(device.last_seen)}`}</span>
    </header>

    <section class="device-summary">
      <dl class="facts">
        <div><dt>HOST</dt><dd>${escapeHTML(device.hostname)}</dd></div>
        <div><dt>NODE</dt><dd id="node-agent">${escapeHTML(device.agent_version)}</dd></div>
        <div><dt>RELAY</dt><dd>${escapeHTML(relay.version)}</dd></div>
        <div><dt>CAPABILITIES</dt><dd id="node-capabilities">${device.capabilities.length ? device.capabilities.map(value => escapeHTML(value.toUpperCase())).join(" · ") : "NONE"}</dd></div>
      </dl>
      <div class="node-actions">
        ${supportsUpdate
          ? '<button id="update-node" class="text-action" type="button">UPDATE NODE</button>'
          : '<button id="copy-update" class="text-action" type="button">COPY UPDATE COMMAND</button>'}
        <p id="update-state" class="meta">${supportsUpdate ? "Stops active processes while restarting." : "This node predates remote updates."}</p>
        ${supportsUpdate ? "" : `<code class="inline-code">${escapeHTML(updateCommand)}</code>`}
      </div>
    </section>

    <section class="content-section process-launch-section">
      <div class="section-heading"><div><p class="eyebrow">NEW PROCESS</p><h2>Start a PTY</h2></div></div>
      <form id="process-launch" class="process-form">
        <label class="cwd-field">Working directory<input id="process-cwd" spellcheck="false" placeholder="~"></label>
        <label class="command-field">Command<input id="process-command" spellcheck="false" value="sh" required></label>
        <button class="primary-button" type="submit" ${device.online && supportsProcess ? "" : "disabled"}>START</button>
      </form>
      <p id="process-error" class="error">${supportsProcess ? "" : "Update this node to start PTY processes."}</p>
    </section>

    <section class="content-section">
      <div class="section-heading"><div><p class="eyebrow">PROCESSES</p><h2>History</h2></div></div>
      <div id="process-list" class="data-list">${processRows(deviceId, processes)}</div>
    </section>
    ${device.role === "owner" || device.role === "member" ? `<section class="content-section danger-section"><p class="eyebrow">DEVICE</p><a class="text-action danger-text" href="/devices/${deviceId}/delete">REMOVE DEVICE</a></section>` : ""}
  </div>`;

  async function refresh() {
    const [{ device: current }, { processes: currentProcesses }] = await Promise.all([
      api<{ device: Device }>(`/api/v1/devices/${deviceId}`),
      api<{ processes: RemoteProcess[] }>(`/api/v1/devices/${deviceId}/processes`),
    ]);
    const status = qs<HTMLElement>("#device-status");
    status.classList.toggle("online", current.online);
    status.textContent = current.online ? "ONLINE" : `LAST SEEN ${relative(current.last_seen)}`;
    qs<HTMLElement>("#node-agent").textContent = current.agent_version;
    qs<HTMLElement>("#node-capabilities").textContent = current.capabilities.length ? current.capabilities.map(value => value.toUpperCase()).join(" · ") : "NONE";
    qs<HTMLButtonElement>('#process-launch button[type="submit"]').disabled = !current.online || !current.capabilities.includes("process");
    const updateButton = document.querySelector<HTMLButtonElement>("#update-node");
    if (updateButton && !updateInFlight) updateButton.disabled = !current.online || !current.capabilities.includes("update");
    qs<HTMLElement>("#process-list").innerHTML = processRows(deviceId, currentProcesses);
  }

  qs<HTMLFormElement>("#process-launch").addEventListener("submit", async event => {
    event.preventDefault();
    qs<HTMLElement>("#process-error").textContent = "";
    const command = qs<HTMLInputElement>("#process-command").value.trim();
    const cwd = qs<HTMLInputElement>("#process-cwd").value.trim();
    try {
      const result = await relayRequest<{ processId: string }>("process.start", { deviceId, command, cwd, cols: 100, rows: 30 });
      await navigate(`/devices/${deviceId}/processes/${result.processId}`);
    } catch (error) { qs<HTMLElement>("#process-error").textContent = error instanceof Error ? error.message : String(error); }
  });

  document.querySelector<HTMLButtonElement>("#update-node")?.addEventListener("click", async event => {
    const button = event.currentTarget as HTMLButtonElement; button.disabled = true; updateInFlight = true;
    qs<HTMLElement>("#update-state").textContent = "Starting update…";
    try {
      await relayRequest("node.update", { deviceId });
      qs<HTMLElement>("#update-state").textContent = "Updating and restarting…";
    } catch (error) {
      qs<HTMLElement>("#update-state").textContent = error instanceof Error ? error.message : String(error);
      button.disabled = false; updateInFlight = false;
    }
  });
  document.querySelector<HTMLButtonElement>("#copy-update")?.addEventListener("click", event => copyText(updateCommand, event.currentTarget as HTMLButtonElement));

  return onRelayEvent(event => {
    if (event.kind === "relay.connected") { void refresh(); return; }
    if (event.deviceId !== deviceId) return;
    if (event.kind === "node.update.error") {
      qs<HTMLElement>("#update-state").textContent = String(event.detail?.error || "Update failed.");
      updateInFlight = false;
      void refresh();
      return;
    }
    if (event.kind === "node.update.ready") { qs<HTMLElement>("#update-state").textContent = "Restarting node…"; return; }
    if (event.kind === "device.online" && updateInFlight) {
      updateInFlight = false;
      qs<HTMLElement>("#update-state").textContent = "Node updated.";
      void refresh();
      return;
    }
    if (event.kind.startsWith("device.") || event.kind.startsWith("process.")) void refresh();
  });
}

export async function renderDeleteDevice(deviceId: string) {
  const { device } = await api<{ device: Device }>(`/api/v1/devices/${deviceId}`);
  qs<HTMLElement>("#page").innerHTML = `<div class="page narrow-form-page">
    <header class="page-header"><div><p class="eyebrow">${escapeHTML(device.workspace_name.toUpperCase())}</p><h1>Remove device</h1></div></header>
    <section class="content-section danger-section">
      <p>Remove <strong>${escapeHTML(device.name)}</strong> from this workspace?</p>
      <p class="page-copy">The node is disconnected and its process history is deleted.</p>
      <div class="actions"><button id="confirm-device-delete" class="text-action danger-text" type="button">REMOVE</button><a class="text-action" href="/devices/${deviceId}">CANCEL</a></div>
      <p id="delete-error" class="error"></p>
    </section>
  </div>`;
  qs<HTMLButtonElement>("#confirm-device-delete").addEventListener("click", async () => {
    try {
      await api(`/api/v1/devices/${deviceId}`, { method: "DELETE", body: "{}" });
      await navigate(`/workspaces/${device.workspace_id}`);
    } catch (error) {
      qs<HTMLElement>("#delete-error").textContent = error instanceof Error ? error.message : String(error);
    }
  });
}
