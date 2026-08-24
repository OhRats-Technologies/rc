import { Elysia } from "elysia";
import { createApiToken, deleteApiToken, listApiTokens } from "../account";
import { deletePasskey, logout, tokenLogin } from "../auth";
import { removeDevice, getDevice } from "../devices";
import { HttpError } from "../errors";
import { checkOrigin, sessionCookie } from "../http-utils";
import { pageContext, safeNext } from "../page-context";
import {
  createEnrollment, createInvite, createWorkspace, deleteWorkspace, joinWorkspace, workspaceDetail, workspaceFor,
} from "../workspaces";
import { accountPage, apiPage } from "../../web/server/pages/account";
import { authPage } from "../../web/server/pages/auth";
import { deleteDevicePage } from "../../web/server/pages/devices";
import { deleteWorkspacePage, newWorkspacePage, workspacePage } from "../../web/server/pages/workspaces";

async function form(request: Request) { return Object.fromEntries(await request.formData()); }

export const pageActions = new Elysia({ name: "relay.page-actions" })
  .onBeforeHandle(({ request }) => { if (!checkOrigin(request)) return new Response("invalid origin", { status: 403 }); })
  .post("/auth/token", async ({ request }) => {
    const input = await form(request), next = safeNext(input.next);
    try {
      const session = await tokenLogin(input.token);
      return new Response(null, { status: 303, headers: { location: next, "set-cookie": sessionCookie(session) } });
    } catch (error) { return authPage("login", { error: error instanceof Error ? error.message : "Sign in failed." }); }
  })
  .post("/account/logout", ({ request }) => {
    logout(request); return new Response(null, { status: 303, headers: { location: "/", "set-cookie": sessionCookie("", 0) } });
  })
  .post("/workspaces", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request);
    try { return Response.redirect(`/workspaces/${createWorkspace(context.user, input.name).id}`, 303); }
    catch (error) { return newWorkspacePage(context.user, context.workspaces, context.sidebar, error instanceof Error ? error.message : "Could not create workspace.", String(input.name || "")); }
  })
  .post("/workspaces/join", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { return Response.redirect(`/workspaces/${joinWorkspace(context.user, (await form(request)).token).workspaceId}`, 303); }
    catch (error) { return authPage("join", { error: error instanceof Error ? error.message : "Could not join workspace." }); }
  })
  .post("/workspaces/:workspaceId/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 });
    try { deleteWorkspace(context.user, workspace.id); return Response.redirect("/workspaces", 303); }
    catch (error) { return deleteWorkspacePage(context.user, context.workspaces, workspace, context.sidebar, error instanceof Error ? error.message : "Delete failed."); }
  })
  .post("/workspaces/:workspaceId/enrollments", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const detail = workspaceDetail(context.user, params.workspaceId); if (!detail) return new Response("not found", { status: 404 });
    try { const result = createEnrollment(context.user, params.workspaceId); return workspacePage(context.user, context.workspaces, detail.workspace, detail.devices, context.sidebar, { kind: "Install", value: result.install }); }
    catch (error) { throw error; }
  })
  .post("/workspaces/:workspaceId/invites", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const detail = workspaceDetail(context.user, params.workspaceId); if (!detail) return new Response("not found", { status: 404 });
    const input = await form(request), result = createInvite(context.user, params.workspaceId, input.role);
    return workspacePage(context.user, context.workspaces, detail.workspace, detail.devices, context.sidebar, { kind: "Invite", value: result.url });
  })
  .post("/devices/:deviceId/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const device = getDevice(context.user, params.deviceId); if (!device) return new Response("not found", { status: 404 });
    try { removeDevice(context.user, device.id); return Response.redirect(`/workspaces/${device.workspace_id}`, 303); }
    catch (error) { return deleteDevicePage(context.user, context.workspaces, device, context.sidebar, error instanceof Error ? error.message : "Remove failed."); }
  })
  .post("/account/passkeys/:id/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { await deletePasskey(request, context.user, params.id); return Response.redirect("/account", 303); }
    catch (error) { return accountPage(context.user, context.workspaces, await import("../auth").then(m => m.listPasskeys(request, context.user)), context.sidebar, error instanceof Error ? error.message : "Remove failed."); }
  })
  .post("/api/tokens", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request);
    try { const created = createApiToken(context.user.id, input.name); return apiPage(context.user, context.workspaces, listApiTokens(context.user.id), context.sidebar, created.token); }
    catch (error) { return apiPage(context.user, context.workspaces, listApiTokens(context.user.id), context.sidebar, "", error instanceof Error ? error.message : "Token creation failed."); }
  })
  .post("/api/tokens/:id/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    if (!deleteApiToken(context.user.id, params.id)) throw new HttpError(404, "token not found");
    return Response.redirect("/api", 303);
  });
