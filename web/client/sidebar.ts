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
  if (open) {
    children.hidden = false; children.style.height = "0px";
    void children.offsetHeight;
    requestAnimationFrame(() => { children.style.height = `${children.scrollHeight}px`; });
    window.setTimeout(() => { if (children.dataset.open === "true") children.style.height = ""; }, 240);
  } else {
    children.style.height = `${children.scrollHeight}px`;
    void children.offsetHeight;
    requestAnimationFrame(() => { children.style.height = "0px"; });
    window.setTimeout(() => { if (children.dataset.open === "false") { children.hidden = true; children.style.height = ""; } }, 240);
  }
}

function animateChildrenResize(folder: HTMLElement, mutate: () => void) {
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  if (children.hidden) { mutate(); return; }
  children.style.height = `${children.scrollHeight}px`; void children.offsetHeight; mutate();
  requestAnimationFrame(() => { children.style.height = `${children.scrollHeight}px`; });
  window.setTimeout(() => { if (!children.hidden) children.style.height = ""; }, 240);
}

document.querySelectorAll<HTMLElement>("[data-workspace-folder]").forEach(folder => {
  const id = folder.dataset.workspaceFolder!, toggle = folder.querySelector<HTMLButtonElement>("[data-workspace-toggle]")!;
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  const head = folder.querySelector<HTMLElement>(".workspace-folder-head")!;
  head.addEventListener("focusin", () => head.classList.add("focused"));
  head.addEventListener("focusout", event => { if (!(event.relatedTarget instanceof Node) || !head.contains(event.relatedTarget)) head.classList.remove("focused"); });
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
  const menu = folder.querySelector<HTMLDetailsElement>(".workspace-menu"), nameView = folder.querySelector<HTMLElement>("[data-workspace-name-view]");
  const rename = folder.querySelector<HTMLFormElement>("[data-workspace-rename-form]");
  const renameInput = rename?.querySelector<HTMLInputElement>('input[name="name"]');
  const cancelRename = () => {
    if (!rename || !nameView || !renameInput) return;
    rename.hidden = true; nameView.hidden = false; folder.classList.remove("editing"); renameInput.value = renameInput.defaultValue;
  };
  folder.querySelector<HTMLButtonElement>("[data-workspace-rename]")?.addEventListener("click", () => {
    if (!rename || !nameView || !renameInput) return;
    menu!.open = false; nameView.hidden = true; rename.hidden = false; folder.classList.add("editing");
    renameInput.focus(); renameInput.select();
  });
  rename?.addEventListener("keydown", event => { if (event.key === "Escape") { event.preventDefault(); cancelRename(); } });
  renameInput?.addEventListener("blur", () => { window.setTimeout(() => { if (document.activeElement !== renameInput) cancelRename(); }); });
});

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
    menu.closest<HTMLElement>(".workspace-folder-head")?.classList.remove("focused");
  });
});
document.addEventListener("keydown", event => { if (event.key === "Escape") document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(menu => menu.open = false); });
