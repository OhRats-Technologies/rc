import { qs } from "./api";
import { navigate, registerRenderer } from "./navigation";
import { renderAccount } from "./pages/account";
import { renderApi } from "./pages/api";
import { renderActivity } from "./pages/activity";
import { renderDeleteDevice, renderDevice, renderDevices } from "./pages/devices";
import { renderProcess } from "./pages/process";
import { renderDeleteWorkspace, renderNewWorkspace, renderWorkspace, renderWorkspaces } from "./pages/workspaces";
import { updateSidebarActive } from "./sidebar";

type Cleanup = void | (() => void);
let cleanup: Cleanup;

function notFound() {
  qs<HTMLElement>("#page").innerHTML = '<div class="page"><header class="page-header"><div><p class="eyebrow">404</p><h1>Not found</h1></div></header></div>';
}

async function renderRoute() {
  if (typeof cleanup === "function") cleanup();
  cleanup = undefined;
  const path = location.pathname;
  if (path === "/" || path === "/devices") cleanup = await renderDevices();
  else if (path === "/workspaces") cleanup = await renderWorkspaces();
  else if (path === "/workspaces/new") cleanup = await renderNewWorkspace();
  else if (path === "/account") cleanup = await renderAccount();
  else if (path === "/api") cleanup = await renderApi();
  else {
    let match = path.match(/^\/devices\/([^/]+)$/);
    if (match) cleanup = await renderDevice(match[1]);
    else if ((match = path.match(/^\/devices\/([^/]+)\/delete$/))) cleanup = await renderDeleteDevice(match[1]);
    else if ((match = path.match(/^\/devices\/([^/]+)\/processes\/([^/]+)$/))) cleanup = await renderProcess(match[1], match[2]);
    else if ((match = path.match(/^\/workspaces\/([^/]+)$/))) cleanup = await renderWorkspace(match[1]);
    else if ((match = path.match(/^\/workspaces\/([^/]+)\/activity$/))) cleanup = await renderActivity(match[1]);
    else if ((match = path.match(/^\/workspaces\/([^/]+)\/delete$/))) cleanup = await renderDeleteWorkspace(match[1]);
    else notFound();
  }
  updateSidebarActive();
  qs<HTMLElement>("#site-content").scrollTo({ top: 0 });
}

export async function startRouter() {
  registerRenderer(renderRoute);
  addEventListener("popstate", () => { renderRoute().catch(console.error); });
  document.addEventListener("click", event => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const anchor = (event.target as Element | null)?.closest<HTMLAnchorElement>("a[href]");
    if (!anchor || anchor.target || anchor.download) return;
    const target = new URL(anchor.href, location.href);
    if (target.origin !== location.origin || target.pathname === "/install.sh" || target.pathname.startsWith("/downloads/")) return;
    event.preventDefault();
    navigate(target.pathname + target.search + target.hash).catch(console.error);
  });
  await renderRoute();
}
