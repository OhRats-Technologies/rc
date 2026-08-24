import { api, escapeHTML, qs } from "./api";
import type { Me } from "./types";

let current: Me | null = null;

function workspaceIdFromPath() { return location.pathname.match(/^\/workspaces\/([^/]+)/)?.[1] || ""; }

export function updateSidebarActive() {
  const path = location.pathname;
  qs<HTMLElement>('[data-nav="devices"]').classList.toggle("active", path.startsWith("/devices"));
  qs<HTMLElement>('[data-nav="workspaces"]').classList.toggle("active", path.startsWith("/workspaces"));
  qs<HTMLElement>('[data-nav="api"]').classList.toggle("active", path === "/api");
  const workspaceId = workspaceIdFromPath();
  document.querySelectorAll<HTMLElement>("[data-workspace]").forEach(link => {
    link.classList.toggle("active", link.dataset.workspace === workspaceId);
  });
}

function render(data: Me) {
  current = data;
  qs<HTMLElement>("#profile-name").textContent = data.user.name;
  qs<HTMLElement>("#profile-initial").textContent = data.user.name.trim().slice(0, 1).toUpperCase() || "?";
  qs<HTMLElement>("#workspace-links").innerHTML = data.workspaces.length ? data.workspaces.map(workspace => `
    <a class="workspace-link" data-workspace="${workspace.id}" href="/workspaces/${workspace.id}">
      <span class="workspace-dot"></span><span>${escapeHTML(workspace.name)}</span>
    </a>`).join("") : '<span class="sidebar-empty">NO WORKSPACES</span>';
  updateSidebarActive();
}

export async function refreshSidebar() { render(await api<Me>("/api/v1/me")); }

export function initializeSidebar(data: Me) {
  const saved = localStorage.getItem("relay-sidebar");
  document.documentElement.dataset.sidebar = matchMedia("(max-width:760px)").matches ? "closed" : (saved || "open");
  render(data);
  if (qs<HTMLElement>("#site-sidebar").dataset.bound) return;
  qs<HTMLElement>("#site-sidebar").dataset.bound = "1";
  qs<HTMLButtonElement>("#sidebar-toggle").addEventListener("click", () => {
    const closed = document.documentElement.dataset.sidebar === "closed";
    document.documentElement.dataset.sidebar = closed ? "open" : "closed";
    localStorage.setItem("relay-sidebar", closed ? "open" : "closed");
  });
  qs<HTMLButtonElement>("#logout").addEventListener("click", async () => {
    await api("/api/v1/auth/logout", { method: "POST", body: "{}" });
    location.href = "/";
  });
}

export function currentAccount() { return current; }
