import { api, copyText, escapeHTML, qs, relative } from "../api";
import { onRelayEvent } from "../events";
import { navigate } from "../navigation";
import { refreshSidebar } from "../sidebar";
import type { Device, Workspace } from "../types";

function deviceRows(devices: Device[]) {
  return devices.length ? devices.map(device => `<a class="data-row" href="/devices/${device.id}">
    <div><strong>${escapeHTML(device.name)}</strong><div class="meta">${escapeHTML(device.platform.toUpperCase())}/${escapeHTML(device.arch)}</div></div>
    <span class="status ${device.online ? "online" : ""}">${device.online ? "ONLINE" : relative(device.last_seen)}</span>
  </a>`).join("") : '<p class="empty-state">No devices in this workspace.</p>';
}

export async function renderWorkspaces() {
  const { workspaces } = await api<{ workspaces: Workspace[] }>("/api/v1/workspaces");
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">WORKSPACES</p><h1>Workspaces</h1></div><a class="or-button" href="/workspaces/new">NEW WORKSPACE</a></header>
    <div class="data-list">${workspaces.length ? workspaces.map(workspace => `<a class="data-row" href="/workspaces/${workspace.id}"><div><strong>${escapeHTML(workspace.name)}</strong><div class="meta">${workspace.role.toUpperCase()}</div></div><span>→</span></a>`).join("") : '<p class="empty-state">No workspaces.</p>'}</div>
  </div>`;
}

export async function renderNewWorkspace() {
  qs<HTMLElement>("#page").innerHTML = `<div class="page narrow-form-page">
    <header class="page-header"><div><p class="eyebrow">WORKSPACES</p><h1>New workspace</h1></div></header>
    <form id="workspace-form" class="simple-form"><label>Name<input name="name" required autofocus></label><button class="or-button" type="submit">CREATE</button><p id="form-error" class="error"></p></form>
  </div>`;
  qs<HTMLFormElement>("#workspace-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    try {
      const name = new FormData(form).get("name");
      const out = await api<{ id: string }>("/api/v1/workspaces", { method: "POST", body: JSON.stringify({ name }) });
      await refreshSidebar();
      await navigate(`/workspaces/${out.id}`);
    } catch (error) { qs<HTMLElement>("#form-error").textContent = error instanceof Error ? error.message : String(error); }
  });
}

export async function renderWorkspace(workspaceId: string) {
  const { workspace, devices } = await api<{ workspace: Workspace; devices: Device[] }>(`/api/v1/workspaces/${workspaceId}`);
  const writable = workspace.role === "owner" || workspace.role === "member";
  qs<HTMLElement>("#page").innerHTML = `<div class="page">
    <header class="page-header"><div><p class="eyebrow">WORKSPACE</p><h1>${escapeHTML(workspace.name)}</h1><p class="meta">${workspace.role.toUpperCase()}</p></div></header>
    <section class="content-section">
      <div class="section-heading"><div><p class="eyebrow">DEVICES</p><h2 id="workspace-device-count">${devices.length} ${devices.length === 1 ? "device" : "devices"}</h2></div>${writable ? '<button id="enroll-device" class="or-button" type="button">ENROLL DEVICE</button>' : ""}</div>
      <div id="workspace-device-list" class="data-list">${deviceRows(devices)}</div>
      <div id="enrollment-result" class="credential-result" hidden></div>
    </section>
    <section class="content-section"><p class="eyebrow">ACTIVITY</p><a class="or-button" href="/workspaces/${workspaceId}/activity">VIEW AUDIT LOG <span aria-hidden="true">→</span></a></section>
    ${workspace.role === "owner" ? `<section class="content-section"><div class="section-heading"><div><p class="eyebrow">INVITE</p><h2>Workspace access</h2></div><button id="create-invite" class="or-button" type="button">CREATE INVITE</button></div><div id="invite-result" class="credential-result" hidden></div></section>
    <section class="content-section danger-section"><p class="eyebrow">DELETE WORKSPACE</p><a class="text-action danger-text" href="/workspaces/${workspaceId}/delete">DELETE WORKSPACE</a></section>` : ""}
  </div>`;

  const refreshDevices = async () => {
    const data = await api<{ devices: Device[] }>(`/api/v1/workspaces/${workspaceId}`);
    qs<HTMLElement>("#workspace-device-count").textContent = `${data.devices.length} ${data.devices.length === 1 ? "device" : "devices"}`;
    qs<HTMLElement>("#workspace-device-list").innerHTML = deviceRows(data.devices);
  };
  document.querySelector<HTMLButtonElement>("#enroll-device")?.addEventListener("click", async () => {
    const out = await api<{ install: string }>(`/api/v1/workspaces/${workspaceId}/enrollments`, { method: "POST", body: "{}" });
    const result = qs<HTMLElement>("#enrollment-result"); result.hidden = false;
    result.innerHTML = `<code>${escapeHTML(out.install)}</code><button id="copy-enroll" class="text-button" type="button">COPY</button>`;
    qs<HTMLButtonElement>("#copy-enroll").addEventListener("click", event => copyText(out.install, event.currentTarget as HTMLButtonElement));
  });
  document.querySelector<HTMLButtonElement>("#create-invite")?.addEventListener("click", async () => {
    const out = await api<{ url: string }>(`/api/v1/workspaces/${workspaceId}/invites`, { method: "POST", body: JSON.stringify({ role: "member" }) });
    const result = qs<HTMLElement>("#invite-result"); result.hidden = false;
    result.innerHTML = `<code>${escapeHTML(out.url)}</code><button id="copy-invite" class="text-button" type="button">COPY</button>`;
    qs<HTMLButtonElement>("#copy-invite").addEventListener("click", event => copyText(out.url, event.currentTarget as HTMLButtonElement));
  });
  return onRelayEvent(event => {
    if (event.workspaceId === workspaceId && event.kind.startsWith("device.")) void refreshDevices();
  });
}

export async function renderDeleteWorkspace(workspaceId: string) {
  const { workspace } = await api<{ workspace: Workspace }>(`/api/v1/workspaces/${workspaceId}`);
  qs<HTMLElement>("#page").innerHTML = `<div class="page narrow-form-page">
    <header class="page-header"><div><p class="eyebrow">${escapeHTML(workspace.name.toUpperCase())}</p><h1>Delete workspace</h1></div></header>
    <section class="content-section danger-section"><p>Delete <strong>${escapeHTML(workspace.name)}</strong> and its devices?</p><div class="actions"><button id="confirm-delete" class="text-action danger-text" type="button">DELETE</button><a class="text-action" href="/workspaces/${workspaceId}">CANCEL</a></div></section>
  </div>`;
  qs<HTMLButtonElement>("#confirm-delete").addEventListener("click", async () => {
    await api(`/api/v1/workspaces/${workspaceId}`, { method: "DELETE", body: "{}" });
    await refreshSidebar();
    await navigate("/workspaces");
  });
}
