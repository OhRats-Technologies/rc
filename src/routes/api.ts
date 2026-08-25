import { Elysia, t } from "elysia";
import { openapi } from "@elysia/openapi";
import type { AuthenticationResponseJSON, RegistrationResponseJSON } from "@simplewebauthn/server";
import { createAction, deleteAction, getAction, listActions, runAction, updateAction } from "../actions";
import { createApiToken, deleteApiToken, listApiTokens } from "../account";
import {
  addPasskeyOptions, auth, deletePasskey, loginOptions, registerOptions, rcStatus,
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
import { checkOrigin, fail, json } from "../http-utils";
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

export const apiRoutes = new Elysia({ name: "rc.api", prefix: "/api/v1" })
  .use(openapi({
    path: "/openapi",
    documentation: {
      info: { title: "RC API", description: "RC HTTP API.", version: VERSION },
      servers: [{ url: PUBLIC_URL }],
      components: { securitySchemes: { bearerAuth: { type: "http", scheme: "bearer", bearerFormat: "RC API token" } } },
      security: [{ bearerAuth: [] }],
    },
  }))
  .derive(async ({ request }) => ({ rcUser: await auth(request) }))
  .onBeforeHandle(({ request, set, rcUser }) => {
    set.headers["cache-control"] = "no-store";
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const path = new URL(request.url).pathname;
    const publicRoute = path === "/api/v1/health" || path === "/api/v1/status" || path.startsWith("/api/v1/agent/")
      || ["/api/v1/auth/setup/options", "/api/v1/auth/setup/verify", "/api/v1/auth/login/options", "/api/v1/auth/login/verify",
        "/api/v1/auth/register/options", "/api/v1/auth/register/verify", "/api/v1/auth/cli/start", "/api/v1/auth/cli/poll"].includes(path);
    if (!publicRoute && !rcUser) return fail("authentication required", 401);
  })
  .onError(({ error, code, status }) => {
    if (error instanceof HttpError) return status(error.status, { error: error.message });
    if (code === "VALIDATION") return status(400, { error: "invalid request" });
  })
  .get("/health", () => ({ ok: true as const, version: VERSION, agents: agentsCount() }), {
    response: t.Object({ ok: t.Literal(true), version: t.String(), agents: t.Number() }),
    detail: { hide: true },
  })
  .get("/status", ({ request }) => rcStatus(request), { detail: { hide: true } })
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
  .delete("/auth/cli/session", ({ request, rcUser }) => {
    if (!rcUser) throw new HttpError(401, "authentication required");
    const token = request.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1] || "";
    if (!token.startsWith("rc_cli_")) throw new HttpError(400, "CLI session required");
    revokeCliToken(token); return { ok: true };
  }, { detail: { hide: true } })
  .post("/auth/register/options", ({ body }) => registerOptions(body.invite, body.name), {
    body: t.Object({ invite: t.String(), name: t.String({ maxLength: 120 }) }), detail: { hide: true },
  })
  .post("/auth/register/verify", ({ body }) => verifyNewUser("register", body.ceremonyId, body.response as RegistrationResponseJSON), {
    body: WebAuthnVerify, detail: { hide: true },
  })
  .post("/agent/enroll", ({ body }) => json(enrollAgent(body as AgentEnrollInput), 201), { body: AgentEnroll, detail: { hide: true } })
  .get("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery, detail: { hide: true } })
  .delete("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery, detail: { hide: true } })
  .get("/me", ({ rcUser }) => ({ user: rcUser!, workspaces: userWorkspaces(rcUser!.id) }))
  .post("/passkeys/options", ({ request, rcUser }) => addPasskeyOptions(request, rcUser!), { body: t.Optional(t.Object({})), detail: { hide: true } })
  .post("/passkeys/verify", async ({ request, rcUser, body }) => {
    await verifyAddedPasskey(request, rcUser!, body.ceremonyId, body.response as RegistrationResponseJSON);
    return json({ ok: true }, 201);
  }, { body: WebAuthnVerify, detail: { hide: true } })
  .delete("/passkeys/:id", async ({ request, rcUser, params }) => {
    await deletePasskey(request, rcUser!, params.id); return { ok: true };
  }, { params: IdParams, detail: { hide: true } })
  .get("/tokens", ({ rcUser }) => ({ tokens: listApiTokens(rcUser!.id) }))
  .post("/tokens", ({ rcUser, body }) => json(createApiToken(rcUser!.id, body.name), 201), {
    body: t.Object({ name: t.Optional(t.String({ maxLength: 80 })) }),
  })
  .delete("/tokens/:id", ({ rcUser, params }) => {
    if (!deleteApiToken(rcUser!.id, params.id)) throw new HttpError(404, "token not found");
    return { ok: true };
  }, { params: IdParams })
  .get("/workspaces", ({ rcUser }) => ({ workspaces: userWorkspaces(rcUser!.id) }))
  .post("/workspaces", ({ rcUser, body }) => json(createWorkspace(rcUser!, body.name), 201), {
    body: t.Object({ name: t.String({ maxLength: 120 }) }),
  })
  .post("/workspaces/join", ({ rcUser, body }) => joinWorkspace(rcUser!, body.token), {
    body: t.Object({ token: t.String() }),
  })
  .get("/workspaces/:workspaceId", ({ rcUser, params }) => {
    const value = workspaceDetail(rcUser!, params.workspaceId);
    if (!value) throw new HttpError(404, "workspace not found");
    return value;
  }, { params: WorkspaceParams })
  .delete("/workspaces/:workspaceId", ({ rcUser, params }) => {
    deleteWorkspace(rcUser!, params.workspaceId); return { ok: true };
  }, { params: WorkspaceParams })
  .patch("/workspaces/:workspaceId", ({ rcUser, params, body }) => renameWorkspace(rcUser!, params.workspaceId, body.name), {
    params: WorkspaceParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }) }),
  })
  .post("/workspaces/:workspaceId/leave", ({ rcUser, params }) => {
    leaveWorkspace(rcUser!, params.workspaceId); return { ok: true };
  }, { params: WorkspaceParams, body: t.Optional(t.Object({})) })
  .get("/workspaces/:workspaceId/activity", ({ rcUser, params }) => ({ events: workspaceActivity(rcUser!, params.workspaceId) }), {
    params: WorkspaceParams,
  })
  .post("/workspaces/:workspaceId/invites", ({ rcUser, params, body }) => json(createInvite(rcUser!, params.workspaceId, body.role), 201), {
    params: WorkspaceParams, body: t.Object({ role: t.Optional(t.Union([t.Literal("operator"), t.Literal("viewer")])) }),
  })
  .get("/workspaces/:workspaceId/access", ({ rcUser, params }) => workspaceAccess(rcUser!, params.workspaceId), { params: WorkspaceParams })
  .patch("/workspaces/:workspaceId/members/:id", ({ rcUser, params, body }) => changeWorkspaceRole(rcUser!, params.workspaceId, params.id, body.role), {
    params: t.Object({ workspaceId: t.String(), id: t.String() }), body: t.Object({ role: t.Union([t.Literal("owner"), t.Literal("operator"), t.Literal("viewer")]) }),
  })
  .delete("/workspaces/:workspaceId/members/:id", ({ rcUser, params }) => {
    removeWorkspaceMember(rcUser!, params.workspaceId, params.id); return { ok: true };
  }, { params: t.Object({ workspaceId: t.String(), id: t.String() }) })
  .delete("/workspaces/:workspaceId/invites/:id", ({ rcUser, params }) => {
    revokeInvite(rcUser!, params.workspaceId, params.id); return { ok: true };
  }, { params: t.Object({ workspaceId: t.String(), id: t.String() }) })
  .post("/workspaces/:workspaceId/enrollments", ({ rcUser, params }) => json(createEnrollment(rcUser!, params.workspaceId), 201), {
    params: WorkspaceParams, body: t.Optional(t.Object({})),
  })
  .get("/devices", ({ rcUser }) => ({ devices: listDevices(rcUser!) }))
  .get("/devices/:deviceId", ({ rcUser, params }) => {
    const device = getDevice(rcUser!, params.deviceId);
    if (!device) throw new HttpError(404, "device not found");
    return { device };
  }, { params: DeviceParams })
  .delete("/devices/:deviceId", ({ rcUser, params }) => {
    removeDevice(rcUser!, params.deviceId); return { ok: true };
  }, { params: DeviceParams })
  .patch("/devices/:deviceId", ({ rcUser, params, body }) => renameDevice(rcUser!, params.deviceId, body.name), {
    params: DeviceParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }) }),
  })
  .get("/devices/:deviceId/processes", ({ rcUser, params }) => ({ processes: listProcesses(rcUser!.id, params.deviceId) }), {
    params: DeviceParams,
  })
  .post("/devices/:deviceId/processes", ({ rcUser, params, body }) => json(startProcess(rcUser!.id, {
    deviceId: params.deviceId, command: body.command, cwd: body.cwd, cols: body.cols || 100, rows: body.rows || 30,
  }), 201), {
    params: DeviceParams, body: t.Object({ command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), cols: t.Optional(t.Number({ minimum: 2, maximum: 500 })), rows: t.Optional(t.Number({ minimum: 2, maximum: 500 })) }),
  })
  .get("/processes/:id", ({ rcUser, params }) => ({ process: getProcess(rcUser!.id, params.id) }), { params: IdParams })
  .get("/actions", ({ rcUser }) => ({ actions: listActions(rcUser!) }))
  .post("/actions", ({ rcUser, body }) => json(createAction(rcUser!, body.workspaceId, body), 201), {
    body: t.Object({ workspaceId: t.String(), name: t.String({ minLength: 1, maxLength: 120 }), description: t.Optional(t.String({ maxLength: 500 })), command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), confirm: t.Optional(t.Boolean()) }),
  })
  .get("/actions/:id", ({ rcUser, params }) => {
    const action = getAction(rcUser!, params.id); if (!action) throw new HttpError(404, "action not found"); return { action };
  }, { params: ActionParams })
  .patch("/actions/:id", ({ rcUser, params, body }) => {
    updateAction(rcUser!, params.id, body); return { ok: true };
  }, { params: ActionParams, body: t.Object({ name: t.String({ minLength: 1, maxLength: 120 }), description: t.Optional(t.String({ maxLength: 500 })), command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), confirm: t.Optional(t.Boolean()) }) })
  .delete("/actions/:id", ({ rcUser, params }) => { deleteAction(rcUser!, params.id); return { ok: true }; }, { params: ActionParams })
  .post("/actions/:id/run", ({ rcUser, params, body }) => ({ results: runAction(rcUser!, params.id, body.deviceIds) }), {
    params: ActionParams, body: t.Object({ deviceIds: t.Array(t.String(), { minItems: 1, maxItems: 100 }) }),
  });
