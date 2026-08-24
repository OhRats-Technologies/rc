import { Elysia, t } from "elysia";
import { openapi } from "@elysia/openapi";
import type { AuthenticationResponseJSON, RegistrationResponseJSON } from "@simplewebauthn/server";
import { createAction, deleteAction, getAction, listActions, runAction, updateAction } from "../actions";
import { createApiToken, deleteApiToken, listApiTokens } from "../account";
import {
  addPasskeyOptions, auth, deletePasskey, listPasskeys, loginOptions, logout, registerOptions, relayStatus,
  setupOptions, verifyAddedPasskey, verifyLogin, verifyNewUser,
} from "../auth";
import { exchangeCliAuthorization, revokeCliToken, startCliAuthorization } from "../cli-auth";
import { PUBLIC_URL, VERSION } from "../config";
import { userWorkspaces } from "../core";
import {
  enrollAgent, getDevice, handleAgentUnregister, listDevices, removeDevice, renameDevice, type AgentEnrollInput,
} from "../devices";
import { HttpError } from "../errors";
import { agentsCount } from "../gateway";
import { checkOrigin, fail, json, sessionCookie } from "../http-utils";
import { getProcess, listProcesses, startProcess } from "../process-api";
import {
  createEnrollment, createInvite, createWorkspace, deleteWorkspace, joinWorkspace, renameWorkspace, workspaceActivity, workspaceDetail,
} from "../workspaces";
import { changeWorkspaceRole, leaveWorkspace, removeWorkspaceMember, revokeInvite, workspaceAccess } from "../workspace-access";

const IdParams = t.Object({ id: t.String({ minLength: 1, maxLength: 100 }) });
const WorkspaceParams = t.Object({ workspaceId: t.String({ minLength: 1, maxLength: 100 }) });
const DeviceParams = t.Object({ deviceId: t.String({ minLength: 1, maxLength: 100 }) });
const ActionParams = t.Object({ id: t.String({ minLength: 1, maxLength: 100 }) });
const WebAuthnVerify = t.Object({ ceremonyId: t.String(), response: t.Unknown() });
const AgentQuery = t.Object({ device: t.String(), ts: t.String(), sig: t.String() });
const AgentEnroll = t.Object({
  token: t.String(), name: t.Optional(t.String()), hostname: t.Optional(t.String()), platform: t.Optional(t.String()),
  arch: t.Optional(t.String()), publicKey: t.String(), agentVersion: t.Optional(t.String()),
  capabilities: t.Optional(t.Array(t.String(), { maxItems: 32 })),
});

export const apiRoutes = new Elysia({ name: "relay.api", prefix: "/api/v1" })
  .use(openapi({
    path: "/openapi",
    documentation: {
      info: { title: "Relay API", description: "Relay HTTP API.", version: VERSION },
      servers: [{ url: PUBLIC_URL }],
      components: { securitySchemes: { bearerAuth: { type: "http", scheme: "bearer", bearerFormat: "Relay API token" } } },
      security: [{ bearerAuth: [] }],
    },
  }))
  .derive(async ({ request }) => ({ relayUser: await auth(request) }))
  .onBeforeHandle(({ request, set, relayUser }) => {
    set.headers["cache-control"] = "no-store";
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const path = new URL(request.url).pathname;
    const publicRoute = path === "/api/v1/health" || path === "/api/v1/status" || path.startsWith("/api/v1/agent/")
      || ["/api/v1/auth/setup/options", "/api/v1/auth/setup/verify", "/api/v1/auth/login/options", "/api/v1/auth/login/verify",
        "/api/v1/auth/register/options", "/api/v1/auth/register/verify", "/api/v1/auth/cli/start", "/api/v1/auth/cli/poll"].includes(path);
    if (!publicRoute && !relayUser) return fail("authentication required", 401);
  })
  .onError(({ error, code, status }) => {
    if (error instanceof HttpError) return status(error.status, { error: error.message });
    if (code === "VALIDATION") return status(400, { error: "invalid request" });
  })
  .get("/health", () => ({ ok: true as const, version: VERSION, agents: agentsCount() }), {
    response: t.Object({ ok: t.Literal(true), version: t.String(), agents: t.Number() }),
    detail: { hide: true },
  })
  .get("/status", ({ request }) => relayStatus(request), { detail: { hide: true } })
  .all("/auth/setup", () => fail("Relay was updated. Refresh this page and try again.", 409), { detail: { hide: true } })
  .all("/auth/login", () => fail("Relay was updated. Refresh this page and try again.", 409), { detail: { hide: true } })
  .all("/auth/register", () => fail("Relay was updated. Refresh this page and try again.", 409), { detail: { hide: true } })
  .post("/auth/setup/options", ({ request, body }) => setupOptions(request, body.name), {
    body: t.Object({ name: t.String({ maxLength: 120 }) }), detail: { hide: true },
  })
  .post("/auth/setup/verify", ({ body }) => verifyNewUser("setup", body.ceremonyId, body.response as RegistrationResponseJSON), {
    body: WebAuthnVerify, detail: { hide: true },
  })
  .post("/auth/login/options", () => loginOptions(), { body: t.Optional(t.Object({})), detail: { hide: true } })
  .post("/auth/login/verify", ({ body }) => verifyLogin(body.ceremonyId, body.response as AuthenticationResponseJSON), {
    body: WebAuthnVerify, detail: { hide: true },
  })
  .post("/auth/cli/start", () => startCliAuthorization(), { body: t.Optional(t.Object({})), detail: { hide: true } })
  .post("/auth/cli/poll", ({ body }) => exchangeCliAuthorization(body.requestId, body.deviceCode), {
    body: t.Object({ requestId: t.String(), deviceCode: t.String() }), detail: { hide: true },
  })
  .delete("/auth/cli/session", ({ request, relayUser }) => {
    if (!relayUser) throw new HttpError(401, "authentication required");
    const token = request.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1] || "";
    if (!token.startsWith("relay_cli_")) throw new HttpError(400, "CLI session required");
    revokeCliToken(token); return { ok: true };
  }, { detail: { hide: true } })
  .post("/auth/register/options", ({ body }) => registerOptions(body.invite, body.name), {
    body: t.Object({ invite: t.String(), name: t.String({ maxLength: 120 }) }), detail: { hide: true },
  })
  .post("/auth/register/verify", ({ body }) => verifyNewUser("register", body.ceremonyId, body.response as RegistrationResponseJSON), {
    body: WebAuthnVerify, detail: { hide: true },
  })
  .post("/auth/logout", ({ request }) => { logout(request); return json({ ok: true }, 200, { "set-cookie": sessionCookie("", 0) }); }, { detail: { hide: true } })
  .post("/agent/enroll", ({ body }) => json(enrollAgent(body as AgentEnrollInput), 201), { body: AgentEnroll, detail: { hide: true } })
  .get("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery, detail: { hide: true } })
  .delete("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery, detail: { hide: true } })
  .get("/me", ({ relayUser }) => ({ user: relayUser!, workspaces: userWorkspaces(relayUser!.id) }))
  .get("/passkeys", ({ request, relayUser }) => listPasskeys(request, relayUser!).then(passkeys => ({ passkeys })), { detail: { hide: true } })
  .post("/passkeys/options", ({ request, relayUser }) => addPasskeyOptions(request, relayUser!), { body: t.Optional(t.Object({})), detail: { hide: true } })
  .post("/passkeys/verify", async ({ request, relayUser, body }) => {
    await verifyAddedPasskey(request, relayUser!, body.ceremonyId, body.response as RegistrationResponseJSON);
    return json({ ok: true }, 201);
  }, { body: WebAuthnVerify, detail: { hide: true } })
  .delete("/passkeys/:id", async ({ request, relayUser, params }) => {
    await deletePasskey(request, relayUser!, params.id); return { ok: true };
  }, { params: IdParams, detail: { hide: true } })
  .get("/tokens", ({ relayUser }) => ({ tokens: listApiTokens(relayUser!.id) }))
  .post("/tokens", ({ relayUser, body }) => json(createApiToken(relayUser!.id, body.name), 201), {
    body: t.Object({ name: t.Optional(t.String({ maxLength: 80 })) }),
  })
  .delete("/tokens/:id", ({ relayUser, params }) => {
    if (!deleteApiToken(relayUser!.id, params.id)) throw new HttpError(404, "token not found");
    return { ok: true };
  }, { params: IdParams })
  .get("/workspaces", ({ relayUser }) => ({ workspaces: userWorkspaces(relayUser!.id) }))
  .post("/workspaces", ({ relayUser, body }) => json(createWorkspace(relayUser!, body.name), 201), {
    body: t.Object({ name: t.String({ maxLength: 120 }) }),
  })
  .post("/workspaces/join", ({ relayUser, body }) => joinWorkspace(relayUser!, body.token), {
    body: t.Object({ token: t.String() }),
  })
  .get("/workspaces/:workspaceId", ({ relayUser, params }) => {
    const value = workspaceDetail(relayUser!, params.workspaceId);
    if (!value) throw new HttpError(404, "workspace not found");
    return value;
  }, { params: WorkspaceParams })
  .delete("/workspaces/:workspaceId", ({ relayUser, params }) => {
    deleteWorkspace(relayUser!, params.workspaceId); return { ok: true };
  }, { params: WorkspaceParams })
  .patch("/workspaces/:workspaceId", ({ relayUser, params, body }) => renameWorkspace(relayUser!, params.workspaceId, body.name), {
    params: WorkspaceParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }) }),
  })
  .post("/workspaces/:workspaceId/leave", ({ relayUser, params }) => {
    leaveWorkspace(relayUser!, params.workspaceId); return { ok: true };
  }, { params: WorkspaceParams, body: t.Optional(t.Object({})) })
  .get("/workspaces/:workspaceId/activity", ({ relayUser, params }) => ({ events: workspaceActivity(relayUser!, params.workspaceId) }), {
    params: WorkspaceParams,
  })
  .post("/workspaces/:workspaceId/invites", ({ relayUser, params, body }) => json(createInvite(relayUser!, params.workspaceId, body.role), 201), {
    params: WorkspaceParams, body: t.Object({ role: t.Optional(t.Union([t.Literal("operator"), t.Literal("viewer")])) }),
  })
  .get("/workspaces/:workspaceId/access", ({ relayUser, params }) => workspaceAccess(relayUser!, params.workspaceId), { params: WorkspaceParams })
  .patch("/workspaces/:workspaceId/members/:id", ({ relayUser, params, body }) => changeWorkspaceRole(relayUser!, params.workspaceId, params.id, body.role), {
    params: t.Object({ workspaceId: t.String(), id: t.String() }), body: t.Object({ role: t.Union([t.Literal("owner"), t.Literal("operator"), t.Literal("viewer")]) }),
  })
  .delete("/workspaces/:workspaceId/members/:id", ({ relayUser, params }) => {
    removeWorkspaceMember(relayUser!, params.workspaceId, params.id); return { ok: true };
  }, { params: t.Object({ workspaceId: t.String(), id: t.String() }) })
  .delete("/workspaces/:workspaceId/invites/:id", ({ relayUser, params }) => {
    revokeInvite(relayUser!, params.workspaceId, params.id); return { ok: true };
  }, { params: t.Object({ workspaceId: t.String(), id: t.String() }) })
  .post("/workspaces/:workspaceId/enrollments", ({ relayUser, params }) => json(createEnrollment(relayUser!, params.workspaceId), 201), {
    params: WorkspaceParams, body: t.Optional(t.Object({})),
  })
  .get("/devices", ({ relayUser }) => ({ devices: listDevices(relayUser!) }))
  .get("/devices/:deviceId", ({ relayUser, params }) => {
    const device = getDevice(relayUser!, params.deviceId);
    if (!device) throw new HttpError(404, "device not found");
    return { device };
  }, { params: DeviceParams })
  .delete("/devices/:deviceId", ({ relayUser, params }) => {
    removeDevice(relayUser!, params.deviceId); return { ok: true };
  }, { params: DeviceParams })
  .patch("/devices/:deviceId", ({ relayUser, params, body }) => renameDevice(relayUser!, params.deviceId, body.name), {
    params: DeviceParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }) }),
  })
  .get("/devices/:deviceId/processes", ({ relayUser, params }) => ({ processes: listProcesses(relayUser!.id, params.deviceId) }), {
    params: DeviceParams,
  })
  .post("/devices/:deviceId/processes", ({ relayUser, params, body }) => json(startProcess(relayUser!.id, {
    deviceId: params.deviceId, command: body.command, cwd: body.cwd, cols: body.cols || 100, rows: body.rows || 30,
  }), 201), {
    params: DeviceParams, body: t.Object({ command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), cols: t.Optional(t.Number({ minimum: 2, maximum: 500 })), rows: t.Optional(t.Number({ minimum: 2, maximum: 500 })) }),
  })
  .get("/processes/:id", ({ relayUser, params }) => ({ process: getProcess(relayUser!.id, params.id) }), { params: IdParams })
  .get("/actions", ({ relayUser }) => ({ actions: listActions(relayUser!) }))
  .post("/actions", ({ relayUser, body }) => json(createAction(relayUser!, body.workspaceId, body), 201), {
    body: t.Object({ workspaceId: t.String(), name: t.String({ minLength: 1, maxLength: 120 }), description: t.Optional(t.String({ maxLength: 500 })), command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), confirm: t.Optional(t.Boolean()) }),
  })
  .get("/actions/:id", ({ relayUser, params }) => {
    const action = getAction(relayUser!, params.id); if (!action) throw new HttpError(404, "action not found"); return { action };
  }, { params: ActionParams })
  .patch("/actions/:id", ({ relayUser, params, body }) => {
    updateAction(relayUser!, params.id, body); return { ok: true };
  }, { params: ActionParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }), description: t.Optional(t.String({ maxLength: 500 })), command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), confirm: t.Optional(t.Boolean()) }) })
  .delete("/actions/:id", ({ relayUser, params }) => { deleteAction(relayUser!, params.id); return { ok: true }; }, { params: ActionParams })
  .post("/actions/:id/run", ({ relayUser, params, body }) => ({ results: runAction(relayUser!, params.id, body.deviceIds) }), {
    params: ActionParams, body: t.Object({ deviceIds: t.Array(t.String(), { minItems: 1, maxItems: 100 }) }),
  });
