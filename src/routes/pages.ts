import { Elysia } from "elysia";
import { listApiTokens } from "../account";
import { listPasskeys, publicSignupAvailable, rcStatus } from "../auth";
import { cliAuthorizationPreview } from "../cli-auth";
import { PUBLIC_URL, SETUP_TOKEN } from "../config";
import { sha } from "../db";
import { getDevice, listDevices } from "../devices";
import { HttpError } from "../errors";
import { fail, setupCookie } from "../http-utils";
import { pageContext, safeNext } from "../page-context";
import { getProcess, listProcesses } from "../process-api";
import { invitePreview, workspaceActivity, workspaceFor } from "../workspaces";
import { workspaceAccess } from "../workspace-access";
import { activeUserCount } from "../users";
import { accountPage, apiKeyFormPage, apiPage, deleteAccountFallbackPage } from "../../web/server/pages/account";
import { accessPage } from "../../web/server/pages/access";
import { authPage, cliLoginPage, notFoundPage } from "../../web/server/pages/auth";
import { landingPage } from "../../web/server/pages/landing";
import { openapiReferencePage } from "../../web/server/pages/openapi";
import { docsPage } from "../../web/server/pages/docs";
import { apiArticle } from "../../web/server/docs/api";
import { authenticationArticle } from "../../web/server/docs/authentication";
import { cliArticle } from "../../web/server/docs/cli";
import { mcpArticle } from "../../web/server/docs/mcp";
import { principlesArticle } from "../../web/server/docs/principles";
import { quickstartArticle } from "../../web/server/docs/quickstart";
import { securityArticle } from "../../web/server/docs/security";
import { devicePage, devicesPage } from "../../web/server/pages/devices";
import { enrollDevicePage } from "../../web/server/pages/enroll";
import { processPage } from "../../web/server/pages/process";
import { activityPage } from "../../web/server/pages/workspaces";

const loginRedirect = () => Response.redirect("/login", 303);

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
    if (!context && invite) return authPage(query.signin !== "1" ? "register" : "login", { invite, workspaceName: preview?.workspaceName, role: preview?.role, next });
    if (!context && next !== "/devices") return authPage("login", { next });
    if (!context) return landingPage();
    if (invite) return authPage("join", { invite, workspaceName: preview!.workspaceName, role: preview!.role });
    return Response.redirect("/devices", 303);
  })
  .get("/login", async ({ request, query }) => {
    const context = await pageContext(request); if (context) return Response.redirect("/devices", 303);
    return authPage("login", { next: safeNext(query.next) });
  })
  .get("/signup", async ({ request }) => {
    if (!publicSignupAvailable()) return Response.redirect("/login", 303);
    const context = await pageContext(request); if (context) return Response.redirect("/devices", 303);
    return authPage("signup");
  })
  .get("/docs", () => docsPage(quickstartArticle()))
  .get("/docs/quickstart", () => Response.redirect("/docs", 308))
  .get("/docs/principles", () => docsPage(principlesArticle()))
  .get("/docs/security", () => docsPage(securityArticle()))
  .get("/docs/authentication", () => docsPage(authenticationArticle()))
  .get("/docs/cli", () => docsPage(cliArticle()))
  .get("/docs/mcp", () => docsPage(mcpArticle()))
  .get("/docs/api", () => docsPage(apiArticle()))
  .get("/api/v1/openapi", () => openapiReferencePage())
  .get("/cli/login", async ({ request, query }) => {
    const code = String(query.code || "");
    const approval = cliAuthorizationPreview(code);
    if (!approval) return new Response("CLI authorization expired", { status: 410, headers: { "cache-control": "no-store" } });
    const context = await pageContext(request);
    if (!context) return Response.redirect(`/?next=${encodeURIComponent(`/cli/login?code=${encodeURIComponent(code)}`)}`, 303);
    return cliLoginPage(context.user, code, Boolean(approval.approved_at), "", approval.client_id, approval.signing_public_key, approval.session_lifetime);
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
    return deleteAccountFallbackPage(context.user, context.workspaces, context.sidebar);
  })
  .get("/api", async ({ request }) => {
    const context = await pageContext(request); if (!context) return loginRedirect();
    return apiPage(context.user, context.workspaces, listApiTokens(context.user.id), context.sidebar);
  });
