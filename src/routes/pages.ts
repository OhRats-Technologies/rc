import { Elysia } from "elysia";
import { getAction, listActions } from "../actions";
import { listApiTokens } from "../account";
import { listPasskeys, rcStatus } from "../auth";
import { cliAuthorizationPreview } from "../cli-auth";
import { PUBLIC_URL, SETUP_TOKEN } from "../config";
import { sha } from "../db";
import { getDevice, listDevices } from "../devices";
import { HttpError } from "../errors";
import { fail, setupCookie } from "../http-utils";
import { pageContext, safeNext } from "../page-context";
import { getProcess, listProcesses } from "../process-api";
import { invitePreview, workspaceActivity, workspaceDevices, workspaceFor } from "../workspaces";
import { workspaceAccess } from "../workspace-access";
import { activeUserCount } from "../users";
import { accountPage, apiKeyFormPage, apiPage, deleteAccountPage } from "../../web/server/pages/account";
import { accessPage } from "../../web/server/pages/access";
import { actionFormPage, actionPage, actionsPage } from "../../web/server/pages/actions";
import { authPage, cliLoginPage, notFoundPage } from "../../web/server/pages/auth";
import { devicePage, devicesPage } from "../../web/server/pages/devices";
import { enrollDevicePage } from "../../web/server/pages/enroll";
import { processPage } from "../../web/server/pages/process";
import { activityPage } from "../../web/server/pages/workspaces";

const loginRedirect = () => Response.redirect("/", 303);

export const pageRoutes = new Elysia({ name: "rc.pages", detail: { hide: true } })
  .get("/setup/:token", ({ params }) => {
    if (activeUserCount() > 0) return Response.redirect(PUBLIC_URL + "/", 303);
    if (!SETUP_TOKEN || sha(params.token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
    return new Response(null, { status: 303, headers: { location: "/", "set-cookie": setupCookie(params.token), "cache-control": "no-store" } });
  })
  .get("/", async ({ request, query }) => {
    const status = rcStatus(request), context = await pageContext(request), invite = String(query.invite || ""), next = safeNext(query.next);
    if (status.setupRequired) return authPage("setup", { authorized: status.setupAuthorized });
    const preview = invite ? invitePreview(invite) : null;
    if (invite && !preview) return authPage("invalid-invite");
    if (!context) return authPage(invite && query.signin !== "1" ? "register" : "login", {
      invite, workspaceName: preview?.workspaceName, role: preview?.role, next,
    });
    if (invite) return authPage("join", { invite, workspaceName: preview!.workspaceName, role: preview!.role });
    return Response.redirect("/devices", 303);
  })
  .get("/cli/login", async ({ request, query }) => {
    const code = String(query.code || "");
    const approval = cliAuthorizationPreview(code);
    if (!approval) return new Response("CLI authorization expired", { status: 410, headers: { "cache-control": "no-store" } });
    const context = await pageContext(request);
    if (!context) return Response.redirect(`/?next=${encodeURIComponent(`/cli/login?code=${encodeURIComponent(code)}`)}`, 303);
    return cliLoginPage(context.user, code, Boolean(approval.approved_at));
  })
  .get("/devices", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return devicesPage(context.user, context.workspaces, listDevices(context.user), context.sidebar);
  })
  .get("/devices/enroll", async ({ request, query }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspaceId = String(query.workspace || "");
    const workspace = workspaceId ? workspaceFor(context.user, workspaceId) : null;
    return enrollDevicePage(context.user, context.workspaces, context.sidebar, "", workspace?.role === "owner" ? workspace.id : "");
  })
  .get("/devices/:deviceId", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const device = getDevice(context.user, params.deviceId); if (!device) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return devicePage(context.user, context.workspaces, device, device.role === "viewer" ? [] : listProcesses(context.user.id, device.id), context.sidebar);
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
  .get("/workspaces/:workspaceId/access", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace || workspace.role !== "owner") return notFoundPage(context.user, context.workspaces, context.sidebar);
    const access = workspaceAccess(context.user, workspace.id);
    return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar);
  })
  .get("/workspaces/:workspaceId/activity", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return activityPage(context.user, context.workspaces, workspace, workspaceActivity(context.user, workspace.id), context.sidebar);
  })
  .get("/account", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return accountPage(context.user, context.workspaces, await listPasskeys(request, context.user), context.sidebar);
  })
  .get("/api/keys/new", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return apiKeyFormPage(context.user, context.workspaces, context.sidebar);
  })
  .get("/account/delete", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return deleteAccountPage(context.user, context.workspaces, context.sidebar);
  })
  .get("/api", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return apiPage(context.user, context.workspaces, listApiTokens(context.user.id), context.sidebar);
  })
  .get("/actions", async ({ request, query }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspaceId = String(query.workspace || "");
    return actionsPage(context.user, context.workspaces, listActions(context.user, workspaceId || undefined), context.sidebar);
  })
  .get("/actions/new", async ({ request, query }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const workspaceId = String(query.workspace || ""), processId = String(query.process || "");
    const workspace = workspaceId ? workspaceFor(context.user, workspaceId) : null;
    if (workspaceId && (!workspace || workspace.role !== "owner")) return notFoundPage(context.user, context.workspaces, context.sidebar);
    let prefill: { workspaceId?: string; name?: string; command?: string; cwd?: string } = { workspaceId };
    if (processId) try {
      const process = getProcess(context.user.id, processId);
      const device = getDevice(context.user, process.device_id);
      if (device?.role === "owner") prefill = { workspaceId: device.workspace_id, name: process.command.slice(0, 80), command: process.command, cwd: process.cwd || "" };
    } catch {}
    return actionFormPage(context.user, context.workspaces, context.sidebar, null, prefill);
  })
  .get("/actions/:id", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const action = getAction(context.user, params.id); if (!action) return notFoundPage(context.user, context.workspaces, context.sidebar);
    return actionPage(context.user, context.workspaces, action, workspaceDevices(action.workspace_id), context.sidebar);
  })
  .get("/actions/:id/edit", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    const action = getAction(context.user, params.id); if (!action || action.role !== "owner") return notFoundPage(context.user, context.workspaces, context.sidebar);
    return actionFormPage(context.user, context.workspaces, context.sidebar, action);
  });
