import { Elysia } from "elysia";
import { deleteApiToken } from "../account";
import { deletePasskey, logout } from "../auth";
import { approveCliAuthorization } from "../cli-auth";
import { renameDevice } from "../devices";
import { HttpError } from "../errors";
import { checkOrigin, sessionCookie } from "../http-utils";
import { pageContext, safeNext } from "../page-context";
import { consumeStepUp } from "../step-up";
import { createEnrollment, createInvite, createWorkspace, joinWorkspace, renameWorkspace, workspaceFor } from "../workspaces";
import { changeWorkspaceRole, leaveWorkspace, removeWorkspaceMember, revokeInvite, workspaceAccess } from "../workspace-access";
import { deleteUser, renameUser } from "../users";
import { accountPage, apiKeyFormPage, apiPage, deleteAccountFallbackPage } from "../../web/server/pages/account";
import { accessPage } from "../../web/server/pages/access";
import { authPage, cliLoginPage } from "../../web/server/pages/auth";
import { enrollDevicePage } from "../../web/server/pages/enroll";

async function form(request: Request) { return Object.fromEntries(await request.formData()); }
const lockClientRequired = () => new Response("JavaScript and passkey authorization are required for RC Lock authority changes.", {
  status: 409, headers: { "cache-control": "no-store" },
});

export const pageActions = new Elysia({ name: "rc.page-actions", detail: { hide: true } })
  .onBeforeHandle(({ request }) => { if (!checkOrigin(request)) return new Response("invalid origin", { status: 403 }); })
  .post("/cli/login", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return lockClientRequired();
  })
  .post("/account/logout", ({ request }) => {
    logout(request); return new Response(null, { status: 303, headers: { location: "/", "set-cookie": sessionCookie("", 0) } });
  })
  .post("/account/name", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try {
      const result = renameUser(context.user, (await form(request)).name);
      if (request.headers.get("accept")?.includes("application/json")) return Response.json(result, { headers: { "cache-control": "no-store" } });
      return Response.redirect("/account", 303);
    }
    catch (error) {
      if (request.headers.get("accept")?.includes("application/json")) return Response.json({ error: error instanceof Error ? error.message : "Rename failed." }, { status: 400, headers: { "cache-control": "no-store" } });
      return accountPage(context.user, context.workspaces, await import("../auth").then(m => m.listPasskeys(request, context.user)), context.sidebar, "", error instanceof Error ? error.message : "Rename failed.");
    }
  })
  .post("/account/delete", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try {
      consumeStepUp(request, context.user);
      deleteUser(context.user);
      if (request.headers.get("accept")?.includes("application/json")) {
        return Response.json({ ok: true }, { headers: { "set-cookie": sessionCookie("", 0), "cache-control": "no-store" } });
      }
      return new Response(null, { status: 303, headers: { location: "/", "set-cookie": sessionCookie("", 0) } });
    } catch (error) {
      if (request.headers.get("accept")?.includes("application/json")) {
        return Response.json({ error: error instanceof Error ? error.message : "Could not delete account." }, { status: 409, headers: { "cache-control": "no-store" } });
      }
      return deleteAccountFallbackPage(context.user, context.workspaces, context.sidebar, error instanceof Error ? error.message : "Could not delete account.");
    }
  })
  .post("/workspaces", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request);
    createWorkspace(context.user, input.name);
    return Response.redirect(safeNext(input.next || "/devices"), 303);
  })
  .post("/workspaces/join", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { joinWorkspace(context.user, (await form(request)).token); return Response.redirect("/devices", 303); }
    catch (error) { return authPage("join", { error: error instanceof Error ? error.message : "Could not join workspace." }); }
  })
  .post("/workspaces/:workspaceId/rename", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace || workspace.role !== "owner") return new Response("not found", { status: 404 });
    const input = await form(request); renameWorkspace(context.user, params.workspaceId, input.name);
    return Response.redirect(safeNext(input.next || "/devices"), 303);
  })
  .post("/workspaces/:workspaceId/leave", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { leaveWorkspace(context.user, params.workspaceId); return Response.redirect("/devices", 303); }
    catch (error) { return new Response(error instanceof Error ? error.message : "Could not leave workspace.", { status: 409 }); }
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
      return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, { scope: "invite", message: error instanceof Error ? error.message : "Invite failed." });
    }
  })
  .post("/workspaces/:workspaceId/invites/:inviteId/revoke", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    try { revokeInvite(context.user, params.workspaceId, params.inviteId); return Response.redirect(`/workspaces/${params.workspaceId}/access`, 303); }
    catch (error) { const workspace = workspaceFor(context.user, params.workspaceId); if (!workspace) return new Response("not found", { status: 404 }); const access = workspaceAccess(context.user, params.workspaceId); return accessPage(context.user, context.workspaces, workspace, access.members, access.invites, context.sidebar, null, { scope: "pending", message: error instanceof Error ? error.message : "Could not revoke invite." }); }
  })
  .post("/workspaces/:workspaceId/members/:memberId/role", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return lockClientRequired();
  })
  .post("/workspaces/:workspaceId/members/:memberId/remove", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return lockClientRequired();
  })
  .post("/devices/:deviceId/rename", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    const input = await form(request); renameDevice(context.user, params.deviceId, input.name);
    return Response.redirect(safeNext(input.next || `/devices/${params.deviceId}`), 303);
  })
  .post("/account/passkeys/:id/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return lockClientRequired();
  })
  .post("/api/tokens", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return apiKeyFormPage(context.user, context.workspaces, context.sidebar, "", "API signing keys are generated in your browser or RC CLI; JavaScript is required here.");
  })
  .post("/api/tokens/:id/delete", async ({ request, params }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return lockClientRequired();
  });
