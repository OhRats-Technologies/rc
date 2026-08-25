const root = document.documentElement;
document.querySelector<HTMLButtonElement>("#sidebar-toggle")?.addEventListener("click", () => {
  const next = root.dataset.sidebar === "closed" ? "open" : "closed";
  root.dataset.sidebar = next;
  document.cookie = `rc_sidebar=${next}; Path=/; SameSite=Lax; Max-Age=31536000${location.protocol === "https:" ? "; Secure" : ""}`;
});

if (matchMedia("(max-width:760px)").matches && !document.cookie.includes("rc_sidebar=")) root.dataset.sidebar = "closed";

document.querySelectorAll<HTMLElement>("[data-workspace-folder]").forEach(folder => {
  const id = folder.dataset.workspaceFolder!, toggle = folder.querySelector<HTMLButtonElement>("[data-workspace-toggle]")!;
  const children = folder.querySelector<HTMLElement>("[data-workspace-children]")!;
  if (folder.dataset.defaultOpen !== "true" && localStorage.getItem(`rc_workspace_${id}`) === "open") {
    children.hidden = false; toggle.ariaExpanded = "true";
  }
  toggle.addEventListener("click", () => {
    children.hidden = !children.hidden; toggle.ariaExpanded = String(!children.hidden);
    localStorage.setItem(`rc_workspace_${id}`, children.hidden ? "closed" : "open");
  });
  folder.querySelector<HTMLButtonElement>("[data-workspace-show-more]")?.addEventListener("click", event => {
    const button = event.currentTarget as HTMLButtonElement, expanded = button.dataset.expanded === "true";
    folder.querySelectorAll<HTMLElement>(".workspace-device-overflow").forEach(item => item.hidden = expanded);
    button.dataset.expanded = expanded ? "false" : "true"; button.textContent = expanded ? "Show more" : "Show less";
  });
});

document.querySelectorAll<HTMLDetailsElement>(".workspace-menu").forEach(menu => menu.addEventListener("toggle", () => {
  if (!menu.open) return;
  document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(other => { if (other !== menu) other.open = false; });
}));
document.addEventListener("keydown", event => { if (event.key === "Escape") document.querySelectorAll<HTMLDetailsElement>(".workspace-menu[open]").forEach(menu => menu.open = false); });
