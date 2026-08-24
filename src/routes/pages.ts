import { Elysia } from "elysia";
import { listApiTokens } from "../account";
import { listPasskeys, relayStatus } from "../auth";
import { PUBLIC_URL, SETUP_TOKEN, VERSION } from "../config";
import { q, sha } from "../db";
import { getDevice, listDevices } from "../devices";
import { HttpError } from "../errors";
import { fail, setupCookie } from "../http-utils";
import { pageContext } from "../page-context";
import { getProcess, listProcesses } from "../process-api";
import { workspaceActivity, workspaceDetail, workspaceFor } from "../workspaces";
import { accountPage, apiPage } from "../../web/server/pages/account";
import { authPage, notFoundPage } from "../../web/server/pages/auth";
import { deleteDevicePage, devicePage, devicesPage } from "../../web/server/pages/devices";
import { processPage } from "../../web/server/pages/process";
import {
  activityPage, deleteWorkspacePage, newWorkspacePage, workspacePage, workspacesPage,
} from "../../web/server/pages/workspaces";

const loginRedirect = () => Response.redirect("/", 303);

export const pageRoutes = new Elysia({ name: "relay.pages", detail: { hide: true } })
  .get("/setup/:token", ({ params }) => {
    if ((q<{ count: number }>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return Response.redirect(PUBLIC_URL + "/", 303);
    if (!SETUP_TOKEN || sha(params.token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
    return new Response(null, { status: 303, headers: { location: "/", "set-cookie": setupCookie(params.token), "cache-control": "no-store" } });
  })
  .get("/", async ({ request, query }) => {
    const status = relayStatus(request), context = await pageContext(request), invite = String(query.invite || "");
    if (status.setupRequired) return authPage("setup", { authorized: status.setupAuthorized });
    if (!context) return authPage(invite && query.signin !== "1" ? "register" : "login", { invite });
    if (invite) return authPage("join", { invite });
    return Response.redirect("/devices", 303);
  })
  .get("/devices", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return devicesPage(context.user, context.workspaces, listDevices(context.user), context.sidebar);
  })
  .get("/devices/:deviceId", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const device = getDevice(context.user, params.deviceId); if (!device) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return devicePage(context.user, context.workspaces, device, listProcesses(context.user.id, device.id), VERSION, context.sidebar);
  })
  .get("/devices/:deviceId/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const device = getDevice(context.user, params.deviceId); if (!device) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return deleteDevicePage(context.user, context.workspaces, device, context.sidebar);
  })
  .get("/devices/:deviceId/processes/:processId", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const device = getDevice(context.user, params.deviceId); if (!device) return notFoundPage(context.user, context.workspaces, context.sidebar);
    try {
      const process = getProcess(context.user.id, params.processId);
      if (process.device_id !== device.id) throw new HttpError(404, "process not found");
      return processPage(context.user, context.workspaces, device, process, context.sidebar);
    } catch { return notFoundPage(context.user, context.workspaces, context.sidebar); }
  })
  .get("/workspaces", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return workspacesPage(context.user, context.workspaces, context.sidebar);
  })
  .get("/workspaces/new", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return newWorkspacePage(context.user, context.workspaces, context.sidebar);
  })
  .get("/workspaces/:workspaceId", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const detail = workspaceDetail(context.user, params.workspaceId); if (!detail) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return workspacePage(context.user, context.workspaces, detail.workspace, detail.devices, context.sidebar);
  })
  .get("/workspaces/:workspaceId/activity", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return activityPage(context.user, context.workspaces, workspace, workspaceActivity(context.user, workspace.id), context.sidebar);
  })
  .get("/workspaces/:workspaceId/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return deleteWorkspacePage(context.user, context.workspaces, workspace, context.sidebar);
  })
  .get("/account", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return accountPage(context.user, context.workspaces, await listPasskeys(request, context.user), context.sidebar);
  })
  .get("/api", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return apiPage(context.user, context.workspaces, listApiTokens(context.user.id), context.sidebar);
  });
