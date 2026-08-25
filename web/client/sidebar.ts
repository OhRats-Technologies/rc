import { onEvent, request } from "./socket";

const root = document.documentElement;
document.querySelector<HTMLButtonElement>("#sidebar-toggle")?.addEventListener("click", () => {
  const next = root.dataset.sidebar === "closed" ? "open" : "closed";
  root.dataset.sidebar = next;
  document.cookie = `rc_sidebar=${next}; Path=/; SameSite=Lax; Max-Age=31536000${location.protocol === "https:" ? "; Secure" : ""}`;
});

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

function setNameMarquee(head: HTMLElement, active: boolean) {
  const viewport = head.querySelector<HTMLElement>("[data-sidebar-name-viewport]");
  const text = viewport?.querySelector<HTMLElement>("[data-sidebar-name-text]");
  if (!viewport || !text) return;
  viewport.classList.remove("marquee");
  viewport.style.removeProperty("--sidebar-marquee-distance");
  if (!active) return;
  requestAnimationFrame(() => {
    const style = getComputedStyle(viewport);
    const available = viewport.clientWidth - parseFloat(style.paddingInlineStart || "0") - parseFloat(style.paddingInlineEnd || "0");
    const distance = Math.ceil(text.scrollWidth - available);
    if (distance > 1) {
      viewport.style.setProperty("--sidebar-marquee-distance", `${distance}px`);
      viewport.classList.add("marquee");
    }
  });
}

function setupCompositeHead(head: HTMLElement) {
  head.addEventListener("pointerenter", () => setNameMarquee(head, true));
  head.addEventListener("pointerleave", () => { if (!head.matches(":focus-within")) setNameMarquee(head, false); });
  head.addEventListener("focusin", () => { head.classList.add("focused"); setNameMarquee(head, true); });
  head.addEventListener("focusout", event => {
    if ((event.relatedTarget instanceof Node) && head.contains(event.relatedTarget)) return;
    head.classList.remove("focused");
    if (!head.matches(":hover")) setNameMarquee(head, false);
  });
}

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
    if (head) setNameMarquee(head, false);
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
