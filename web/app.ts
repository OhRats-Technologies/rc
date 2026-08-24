import { api, qs } from "./api";
import { bindAuth, isUnauthenticated, showAuth } from "./auth";
import { startEvents } from "./events";
import { startRouter } from "./router";
import { initializeSidebar, refreshSidebar } from "./sidebar";
import type { Me, Status } from "./types";

bindAuth();

async function boot() {
  const status = await api<Status>("/api/v1/status");
  if (status.setupRequired) {
    showAuth("setup");
    if (!status.setupAuthorized) qs<HTMLElement>("#auth-error").textContent = "Open the Relay setup link first.";
    return;
  }
  const invite = new URLSearchParams(location.search).get("invite");
  let me: Me;
  try { me = await api<Me>("/api/v1/me"); }
  catch (error) {
    if (isUnauthenticated(error)) { showAuth(invite ? "register" : "login", invite || ""); return; }
    throw error;
  }
  if (invite) {
    await api("/api/v1/workspaces/join", { method: "POST", body: JSON.stringify({ token: invite }) });
    history.replaceState(null, "", "/devices");
    me = await api<Me>("/api/v1/me");
  }
  initializeSidebar(me);
  startEvents();
  await startRouter();
  qs<HTMLElement>("#auth").hidden = true;
  qs<HTMLElement>("#site-shell").hidden = false;
  document.body.classList.add("authenticated");
  void refreshSidebar();
}

boot().catch(error => {
  console.error(error);
  qs<HTMLElement>("#page").innerHTML = `<div class="page"><p class="error">${error instanceof Error ? error.message : String(error)}</p></div>`;
});
