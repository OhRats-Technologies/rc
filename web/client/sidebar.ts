import { onEvent, request } from "./socket";
import { api } from "./http";

const root = document.documentElement;
function setSidebar(next: "open" | "closed") {
  root.dataset.sidebar = next;
  document.cookie = `rc_sidebar=${next}; Path=/; SameSite=Lax; Max-Age=31536000${location.protocol === "https:" ? "; Secure" : ""}`;
}
document.querySelector<HTMLButtonElement>("#sidebar-toggle")?.addEventListener("click", () => setSidebar(root.dataset.sidebar === "closed" ? "open" : "closed"));

if (matchMedia("(max-width:760px)").matches && !document.cookie.includes("rc_sidebar=")) root.dataset.sidebar = "closed";

function setFolderOpen(folder: HTMLElement, open: boolean, animate = true) {
  const toggle = folder.querySelector<HTMLButtonElement>("[data-workspace-toggle]")!;
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  toggle.ariaExpanded = String(open); children.dataset.open = String(open);
  if (!animate) { children.hidden = !open; children.style.height = ""; return; }
  children.classList.add("animating");
  if (open) {
    children.hidden = false; children.style.height = "0px";
    void children.offsetHeight;
    requestAnimationFrame(() => { children.style.height = `${children.scrollHeight}px`; });
    window.setTimeout(() => { if (children.dataset.open === "true") children.style.height = ""; children.classList.remove("animating"); }, 240);
  } else {
    children.style.height = `${children.scrollHeight}px`;
    void children.offsetHeight;
    requestAnimationFrame(() => { children.style.height = "0px"; });
    window.setTimeout(() => { if (children.dataset.open === "false") { children.hidden = true; children.style.height = ""; } children.classList.remove("animating"); }, 240);
  }
}

function animateChildrenResize(folder: HTMLElement, mutate: () => void) {
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  if (children.hidden) { mutate(); return; }
  children.classList.add("animating"); children.style.height = `${children.scrollHeight}px`; void children.offsetHeight; mutate();
  requestAnimationFrame(() => { children.style.height = `${children.scrollHeight}px`; });
  window.setTimeout(() => { if (!children.hidden) children.style.height = ""; children.classList.remove("animating"); }, 240);
}

function setupInlineRename(scope: HTMLElement, viewSelector: string, formSelector: string, triggerSelector: string) {
  const menu = scope.querySelector<HTMLDetailsElement>(".workspace-menu"), nameView = scope.querySelector<HTMLElement>(viewSelector);
  const rename = scope.querySelector<HTMLFormElement>(formSelector), renameInput = rename?.querySelector<HTMLInputElement>('input[name="name"]');
  const cancel = () => {
    if (!rename || !nameView || !renameInput) return;
    rename.hidden = true; nameView.hidden = false; scope.classList.remove("editing"); renameInput.value = renameInput.defaultValue;
  };
  scope.querySelector<HTMLButtonElement>(triggerSelector)?.addEventListener("click", () => {
    if (!rename || !nameView || !renameInput) return;
    if (menu) menu.open = false; nameView.hidden = true; rename.hidden = false; scope.classList.add("editing");
    renameInput.focus(); renameInput.select();
  });
  rename?.addEventListener("keydown", event => { if (event.key === "Escape") { event.preventDefault(); cancel(); } });
  renameInput?.addEventListener("blur", () => { window.setTimeout(() => { if (document.activeElement !== renameInput) cancel(); }); });
}

function setupCompositeHead(head: HTMLElement) {
  head.addEventListener("focusin", () => head.classList.add("focused"));
  head.addEventListener("focusout", event => {
    if ((event.relatedTarget instanceof Node) && head.contains(event.relatedTarget)) return;
    head.classList.remove("focused");
  });
}

function syncDeviceMarquee(device: HTMLElement) {
  const viewport = device.querySelector<HTMLElement>(".workspace-device-name"), text = viewport?.firstElementChild as HTMLElement | null;
  if (!viewport || !text) return;
  const distance = Math.max(0, text.scrollWidth - viewport.clientWidth);
  device.classList.toggle("marquee-overflow", distance > 1);
  device.style.setProperty("--device-marquee-distance", `${distance}px`);
  device.style.setProperty("--device-marquee-duration", `${Math.max(2.5, distance / 26).toFixed(2)}s`);
}

const workspaceCreateForm = document.querySelector<HTMLFormElement>("[data-workspace-create-form]");
const workspaceCreateInput = workspaceCreateForm?.querySelector<HTMLInputElement>('input[name="name"]');
const workspaceEmpty = document.querySelector<HTMLElement>("[data-workspace-empty]");
function cancelWorkspaceCreate() {
  if (!workspaceCreateForm || !workspaceCreateInput) return;
  workspaceCreateForm.hidden = true; workspaceCreateInput.value = "";
  if (workspaceEmpty) workspaceEmpty.hidden = false;
}
function beginWorkspaceCreate() {
  if (!workspaceCreateForm || !workspaceCreateInput) return;
  setSidebar("open"); workspaceCreateForm.hidden = false;
  if (workspaceEmpty) workspaceEmpty.hidden = true;
  workspaceCreateInput.focus(); workspaceCreateInput.select();
}
document.querySelectorAll<HTMLButtonElement>("[data-workspace-create-trigger]").forEach(button => button.addEventListener("click", beginWorkspaceCreate));
workspaceCreateForm?.addEventListener("submit", event => {
  if (!workspaceCreateInput) return;
  const name = workspaceCreateInput.value.trim();
  if (!name) { event.preventDefault(); cancelWorkspaceCreate(); return; }
  workspaceCreateInput.value = name;
});
workspaceCreateForm?.addEventListener("keydown", event => {
  if (event.key === "Escape") { event.preventDefault(); cancelWorkspaceCreate(); }
});
workspaceCreateInput?.addEventListener("blur", () => window.setTimeout(() => {
  if (document.activeElement !== workspaceCreateInput) cancelWorkspaceCreate();
}));

document.querySelectorAll<HTMLElement>("[data-workspace-folder]").forEach(folder => {
  const id = folder.dataset.workspaceFolder!, toggle = folder.querySelector<HTMLButtonElement>("[data-workspace-toggle]")!;
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  const head = folder.querySelector<HTMLElement>(".workspace-folder-head")!;
  setupCompositeHead(head);
  const stored = localStorage.getItem(`rc_workspace_${id}`), initial = stored !== null ? stored === "open" : folder.dataset.defaultOpen === "true";
  setFolderOpen(folder, initial, false);
  toggle.addEventListener("click", () => {
    const open = Boolean(children.hidden); setFolderOpen(folder, open);
    localStorage.setItem(`rc_workspace_${id}`, open ? "open" : "closed");
  });
  folder.querySelector<HTMLButtonElement>("[data-workspace-show-more]")?.addEventListener("click", event => {
    const button = event.currentTarget as HTMLButtonElement, expanded = button.dataset.expanded === "true";
    animateChildrenResize(folder, () => {
      folder.querySelectorAll<HTMLElement>(".workspace-device-overflow").forEach(item => item.hidden = expanded);
      button.dataset.expanded = expanded ? "false" : "true"; button.textContent = expanded ? "Show more" : "Show less";
    });
  });
  setupInlineRename(folder, "[data-workspace-name-view]", "[data-workspace-rename-form]", "[data-workspace-rename]");
});

document.querySelectorAll<HTMLElement>("[data-sidebar-device]").forEach(device => {
  const head = device.querySelector<HTMLElement>(".workspace-device-head");
  if (head) setupCompositeHead(head);
  setupInlineRename(device, "[data-device-name-view]", "[data-device-rename-form]", "[data-device-rename]");
  const refresh = () => requestAnimationFrame(() => syncDeviceMarquee(device));
  refresh(); head?.addEventListener("pointerenter", refresh); head?.addEventListener("focusin", refresh);
  const name = device.querySelector<HTMLElement>(".workspace-device-name");
  if (name && "ResizeObserver" in window) new ResizeObserver(refresh).observe(name);
});

const deleteDialog = document.querySelector<HTMLDialogElement>("[data-delete-dialog]");
const deleteTitle = deleteDialog?.querySelector<HTMLElement>("[data-delete-title]");
const deleteName = deleteDialog?.querySelector<HTMLElement>("[data-delete-name]");
const deleteDescription = deleteDialog?.querySelector<HTMLElement>("[data-delete-description]");
const deleteError = deleteDialog?.querySelector<HTMLElement>("[data-delete-error]");
const deleteConfirm = deleteDialog?.querySelector<HTMLButtonElement>("[data-delete-confirm]");
let deleteEndpoint = "", deleteRedirect = "/devices", deleteMethod = "DELETE", deleteTrigger: HTMLElement | null = null;

document.querySelectorAll<HTMLElement>("[data-delete-endpoint]").forEach(button => button.addEventListener("click", event => {
  event.preventDefault();
  if (!deleteDialog || !deleteTitle || !deleteName || !deleteConfirm) return;
  const menu = button.closest<HTMLDetailsElement>("details");
  deleteEndpoint = button.dataset.deleteEndpoint || ""; deleteRedirect = button.dataset.deleteRedirect || "/devices"; deleteMethod = button.dataset.deleteMethod || "DELETE"; deleteTrigger = menu?.querySelector<HTMLElement>("summary") || button;
  const kind = button.dataset.deleteKind || "item", name = button.dataset.deleteName || "this item";
  deleteTitle.textContent = `Delete ${kind}?`; deleteName.textContent = name;
  if (deleteDescription) { deleteDescription.textContent = button.dataset.deleteDescription || ""; deleteDescription.hidden = !deleteDescription.textContent; }
  if (deleteError) deleteError.textContent = "";
  if (menu) menu.open = false;
  deleteDialog.showModal();
}));

deleteDialog?.querySelector<HTMLButtonElement>("[data-delete-cancel]")?.addEventListener("click", () => deleteDialog.close());
deleteDialog?.addEventListener("click", event => { if (event.target === deleteDialog) deleteDialog.close(); });
deleteDialog?.addEventListener("close", () => { deleteEndpoint = ""; deleteRedirect = "/devices"; deleteMethod = "DELETE"; deleteTrigger?.focus(); deleteTrigger = null; });
deleteConfirm?.addEventListener("click", async () => {
  if (!deleteEndpoint || !deleteConfirm) return;
  deleteConfirm.disabled = true; deleteConfirm.textContent = "Deleting…";
  try { await api(deleteEndpoint, { method: deleteMethod, headers: { accept: "application/json" } }); location.href = deleteRedirect; }
  catch (error) {
    if (deleteError) deleteError.textContent = error instanceof Error ? error.message : String(error);
    deleteConfirm.disabled = false; deleteConfirm.textContent = "Delete";
  }
});

function deviceOnline(deviceId: string) {
  return document.querySelector<HTMLElement>(`[data-sidebar-device-status="${CSS.escape(deviceId)}"]`)?.classList.contains("online") === true;
}

function resetUpdate(button: HTMLButtonElement, message = "") {
  delete button.dataset.updating; button.textContent = "Update node"; button.disabled = !deviceOnline(button.dataset.sidebarDeviceUpdate || "");
  button.title = message;
}

document.querySelectorAll<HTMLButtonElement>("[data-sidebar-device-update]").forEach(button => button.addEventListener("click", async () => {
  const deviceId = button.dataset.sidebarDeviceUpdate || ""; button.dataset.updating = "true"; button.disabled = true; button.textContent = "Updating node…"; button.title = "";
  try { await request({ type: "node.update", deviceId }); button.textContent = "Restarting node…"; }
  catch (error) { resetUpdate(button, error instanceof Error ? error.message : String(error)); }
}));

document.querySelectorAll<HTMLDetailsElement>(".workspace-menu").forEach(menu => menu.addEventListener("toggle", () => {
  if (!menu.open) return;
  document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(other => { if (other !== menu) other.open = false; });
}));
document.addEventListener("pointerdown", event => {
  document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(menu => {
    if (event.target instanceof Node && menu.contains(event.target)) return;
    menu.open = false;
    const trigger = menu.querySelector<HTMLElement>("summary");
    trigger?.blur();
    const head = menu.closest<HTMLElement>(".workspace-folder-head,.workspace-device-head");
    head?.classList.remove("focused");
  });
});
document.addEventListener("keydown", event => { if (event.key === "Escape") document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(menu => menu.open = false); });

onEvent(event => {
  if (!event.deviceId) return;
  if (event.kind === "device.online" || event.kind === "device.offline") {
    const online = event.kind === "device.online";
    document.querySelectorAll<HTMLElement>(`[data-sidebar-device-status="${CSS.escape(event.deviceId)}"]`).forEach(dot => dot.classList.toggle("online", online));
    document.querySelectorAll<HTMLButtonElement>(`[data-sidebar-device-update="${CSS.escape(event.deviceId)}"]`).forEach(button => {
      if (!button.dataset.updating) button.disabled = !online;
    });
  }
  if (event.kind === "node.update.ready") {
    document.querySelectorAll<HTMLButtonElement>(`[data-sidebar-device-update="${CSS.escape(event.deviceId)}"]`).forEach(button => { button.disabled = true; button.dataset.updating = "true"; button.textContent = "Restarting node…"; });
  }
  if (event.kind === "node.update.complete") {
    document.querySelectorAll<HTMLButtonElement>(`[data-sidebar-device-update="${CSS.escape(event.deviceId)}"]`).forEach(button => button.remove());
  }
  if (event.kind === "node.update.error") {
    const message = String(event.detail?.error || "Update failed.");
    document.querySelectorAll<HTMLButtonElement>(`[data-sidebar-device-update="${CSS.escape(event.deviceId)}"]`).forEach(button => resetUpdate(button, message));
  }
});
