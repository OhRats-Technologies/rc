import { Elysia } from "elysia";
import { createAction, deleteAction, getAction, listActions, runAction, updateAction } from "../actions";
import { createApiToken, deleteApiToken, listApiTokens } from "../account";
import { deletePasskey, logout } from "../auth";
import { approveCliAuthorization } from "../cli-auth";
import { removeDevice, getDevice, renameDevice } from "../devices";
import { HttpError } from "../errors";
import { checkOrigin, sessionCookie } from "../http-utils";
import { pageContext, safeNext } from "../page-context";
import {
  createEnrollment, createInvite, createWorkspace, deleteWorkspace, joinWorkspace, renameWorkspace, workspaceDetail, workspaceDevices, workspaceFor,
} from "../workspaces";
import { changeWorkspaceRole, leaveWorkspace, removeWorkspaceMember, revokeInvite, workspaceAccess } from "../workspace-access";
import { accountPage, apiPage } from "../../web/server/pages/account";
import { accessPage } from "../../web/server/pages/access";
import { actionConfirmPage, actionFormPage, actionPage } from "../../web/server/pages/actions";
import { authPage, cliLoginPage } from "../../web/server/pages/auth";
import { deleteDevicePage } from "../../web/server/pages/devices";
import { enrollDevicePage } from "../../web/server/pages/enroll";
import { deleteWorkspacePage, newWorkspacePage, workspacePage } from "../../web/server/pages/workspaces";

async function form(request: Request) { return Object.fromEntries(await request.formData()); }

export const pageActions = new Elysia({ name: "rc.page-actions", detail: { hide: true } })
  .onBeforeHandle(({ request }) => { if (!checkOrigin(request)) return new Response("invalid origin", { status: 403 }); })
  .post("/cli/login", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request);
    try { approveCliAuthorization(context.user, input.code); return cliLoginPage(context.user, "", true); }
    catch (error) { return cliLoginPage(context.user, String(input.code || ""), false, error instanceof Error ? error.message : "Could not authorize CLI."); }
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
  .post("/workspaces/:workspaceId/rename", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace || workspace.role !== "owner") return new Response("not found", { status: 404 });
    const input = await form(request); renameWorkspace(context.user, params.workspaceId, input.name);
    return Response.redirect(safeNext(input.next || `/workspaces/${params.workspaceId}`), 303);
  })
  .post("/workspaces/:workspaceId/leave", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { leaveWorkspace(context.user, params.workspaceId); return Response.redirect("/workspaces", 303); }
    catch (error) {
      const detail = workspaceDetail(context.user, params.workspaceId); if (!detail) return Response.redirect("/workspaces", 303);
      return workspacePage(context.user, context.workspaces, detail.workspace, detail.devices, listActions(context.user, params.workspaceId), context.sidebar, undefined, error instanceof Error ? error.message : "Could not leave workspace.");
    }
  })
  .post("/workspaces/:workspaceId/enrollments", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const detail = workspaceDetail(context.user, params.workspaceId); if (!detail) return new Response("not found", { status: 404 });
    try { const result = createEnrollment(context.user, params.workspaceId); return workspacePage(context.user, context.workspaces, detail.workspace, detail.devices, listActions(context.user, params.workspaceId), context.sidebar, { kind: "enrollment", install: result.install }); }
    catch (error) { throw error; }
  })
  .post("/devices/enroll", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request), workspaceId = String(input.workspaceId || "");
    try { return enrollDevicePage(context.user, context.workspaces, context.sidebar, createEnrollment(context.user, workspaceId).install, workspaceId); }
    catch (error) { return enrollDevicePage(context.user, context.workspaces, context.sidebar, "", workspaceId, error instanceof Error ? error.message : "Could not create enrollment."); }
  })
  .post("/workspaces/:workspaceId/invites", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 });
    try {
      const input = await form(request), role = input.role === "viewer" ? "viewer" as const : "operator" as const;
      const result = createInvite(context.user, params.workspaceId, role), access = workspaceAccess(context.user, params.workspaceId);
      return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, { url: result.url, role });
    } catch (error) {
      const access = workspaceAccess(context.user, params.workspaceId);
      return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, error instanceof Error ? error.message : "Invite failed.");
    }
  })
  .post("/workspaces/:workspaceId/invites/:inviteId/revoke", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { revokeInvite(context.user, params.workspaceId, params.inviteId); return Response.redirect(`/workspaces/${params.workspaceId}/access`, 303); }
    catch (error) { const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 }); const access = workspaceAccess(context.user, params.workspaceId); return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, error instanceof Error ? error.message : "Could not revoke invite."); }
  })
  .post("/workspaces/:workspaceId/members/:memberId/role", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { changeWorkspaceRole(context.user, params.workspaceId, params.memberId, (await form(request)).role); return Response.redirect(`/workspaces/${params.workspaceId}/access`, 303); }
    catch (error) { const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 }); const access = workspaceAccess(context.user, params.workspaceId); return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, error instanceof Error ? error.message : "Could not change role."); }
  })
  .post("/workspaces/:workspaceId/members/:memberId/remove", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { removeWorkspaceMember(context.user, params.workspaceId, params.memberId); return Response.redirect(`/workspaces/${params.workspaceId}/access`, 303); }
    catch (error) { const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 }); const access = workspaceAccess(context.user, params.workspaceId); return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, error instanceof Error ? error.message : "Could not remove member."); }
  })
  .post("/devices/:deviceId/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const device = getDevice(context.user, params.deviceId); if (!device) return new Response("not found", { status: 404 });
    try { removeDevice(context.user, device.id); return Response.redirect(`/workspaces/${device.workspace_id}`, 303); }
    catch (error) { return deleteDevicePage(context.user, context.workspaces, device, context.sidebar, error instanceof Error ? error.message : "Remove failed."); }
  })
  .post("/devices/:deviceId/rename", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    renameDevice(context.user, params.deviceId, (await form(request)).name); return Response.redirect(`/devices/${params.deviceId}`, 303);
  })
  .post("/actions", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request);
    try { return Response.redirect(`/actions/${createAction(context.user, String(input.workspaceId || ""), input).id}`, 303); }
    catch (error) { return actionFormPage(context.user, context.workspaces, context.sidebar, null, { workspaceId: String(input.workspaceId || ""), name: String(input.name || ""), description: String(input.description || ""), command: String(input.command || ""), cwd: String(input.cwd || ""), confirm: Boolean(input.confirm) }, error instanceof Error ? error.message : "Could not create action."); }
  })
  .post("/actions/:id", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const action = getAction(context.user, params.id); if (!action) return new Response("not found", { status: 404 });
    const input = await form(request);
    try { updateAction(context.user, action.id, input); return Response.redirect(`/actions/${action.id}`, 303); }
    catch (error) { return actionFormPage(context.user, context.workspaces, context.sidebar, action, {}, error instanceof Error ? error.message : "Could not update action."); }
  })
  .post("/actions/:id/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    deleteAction(context.user, params.id); return Response.redirect("/actions", 303);
  })
  .post("/actions/:id/run", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const action = getAction(context.user, params.id); if (!action) return new Response("not found", { status: 404 });
    const data = await request.formData(), deviceIds = data.getAll("deviceId").map(String);
    if (action.confirm && data.get("confirm") !== "1") return actionConfirmPage(context.user, context.workspaces, action, workspaceDevices(action.workspace_id), deviceIds, context.sidebar);
    const results = runAction(context.user, action.id, deviceIds);
    if (results.length === 1 && results[0].processId) return Response.redirect(`/devices/${results[0].deviceId}/processes/${results[0].processId}`, 303);
    return actionPage(context.user, context.workspaces, action, workspaceDevices(action.workspace_id), context.sidebar, results);
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
