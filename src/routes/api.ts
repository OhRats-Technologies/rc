import { Elysia, t } from "elysia";
import type { AuthenticationResponseJSON, RegistrationResponseJSON } from "@simplewebauthn/server";
import { createApiToken, deleteApiToken, listApiTokens } from "../account";
import {
  addPasskeyOptions, auth, deletePasskey, listPasskeys, loginOptions, logout, registerOptions, relayStatus,
  setupOptions, tokenLogin, verifyAddedPasskey, verifyLogin, verifyNewUser,
} from "../auth";
import { VERSION } from "../config";
import { userWorkspaces } from "../core";
import {
  enrollAgent, getDevice, handleAgentUnregister, listDevices, removeDevice, type AgentEnrollInput,
} from "../devices";
import { HttpError } from "../errors";
import { agentsCount } from "../gateway";
import { checkOrigin, fail, json, sessionCookie } from "../http-utils";
import { getProcess, listProcesses } from "../process-api";
import {
  createEnrollment, createInvite, createWorkspace, deleteWorkspace, joinWorkspace, workspaceActivity, workspaceDetail,
} from "../workspaces";

const IdParams = t.Object({ id: t.String({ minLength: 1, maxLength: 100 }) });
const WorkspaceParams = t.Object({ workspaceId: t.String({ minLength: 1, maxLength: 100 }) });
const DeviceParams = t.Object({ deviceId: t.String({ minLength: 1, maxLength: 100 }) });
const WebAuthnVerify = t.Object({ ceremonyId: t.String(), response: t.Unknown() });
const AgentQuery = t.Object({ device: t.String(), ts: t.String(), sig: t.String() });
const AgentEnroll = t.Object({
  token: t.String(), name: t.Optional(t.String()), hostname: t.Optional(t.String()), platform: t.Optional(t.String()),
  arch: t.Optional(t.String()), publicKey: t.String(), agentVersion: t.Optional(t.String()),
  capabilities: t.Optional(t.Array(t.String(), { maxItems: 32 })),
});

export const apiRoutes = new Elysia({ name: "relay.api", prefix: "/api/v1" })
  .derive(async ({ request }) => ({ relayUser: await auth(request) }))
  .onBeforeHandle(({ request, set, relayUser }) => {
    set.headers["cache-control"] = "no-store";
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const path = new URL(request.url).pathname;
    const publicRoute = path === "/api/v1/health" || path === "/api/v1/status"
      || path.startsWith("/api/v1/auth/") || path.startsWith("/api/v1/agent/");
    if (!publicRoute && !relayUser) return fail("authentication required", 401);
  })
  .onError(({ error, code, status }) => {
    if (error instanceof HttpError) return status(error.status, { error: error.message });
    if (code === "VALIDATION") return status(400, { error: "invalid request" });
  })
  .get("/health", () => ({ ok: true as const, version: VERSION, agents: agentsCount() }), {
    response: t.Object({ ok: t.Literal(true), version: t.String(), agents: t.Number() }),
  })
  .get("/status", ({ request }) => relayStatus(request))
  .all("/auth/setup", () => fail("Relay was updated. Refresh this page and try again.", 409))
  .all("/auth/login", () => fail("Relay was updated. Refresh this page and try again.", 409))
  .all("/auth/register", () => fail("Relay was updated. Refresh this page and try again.", 409))
  .post("/auth/setup/options", ({ request, body }) => setupOptions(request, body.name), {
    body: t.Object({ name: t.String({ maxLength: 120 }) }),
  })
  .post("/auth/setup/verify", ({ body }) => verifyNewUser("setup", body.ceremonyId, body.response as RegistrationResponseJSON), {
    body: WebAuthnVerify,
  })
  .post("/auth/login/options", () => loginOptions(), { body: t.Optional(t.Object({})) })
  .post("/auth/login/verify", ({ body }) => verifyLogin(body.ceremonyId, body.response as AuthenticationResponseJSON), {
    body: WebAuthnVerify,
  })
  .post("/auth/token", async ({ body }) => json({ ok: true }, 200, { "set-cookie": sessionCookie(await tokenLogin(body.token)) }), {
    body: t.Object({ token: t.String({ minLength: 1, maxLength: 512 }) }),
  })
  .post("/auth/register/options", ({ body }) => registerOptions(body.invite, body.name), {
    body: t.Object({ invite: t.String(), name: t.String({ maxLength: 120 }) }),
  })
  .post("/auth/register/verify", ({ body }) => verifyNewUser("register", body.ceremonyId, body.response as RegistrationResponseJSON), {
    body: WebAuthnVerify,
  })
  .post("/auth/logout", ({ request }) => { logout(request); return json({ ok: true }, 200, { "set-cookie": sessionCookie("", 0) }); })
  .post("/agent/enroll", ({ body }) => json(enrollAgent(body as AgentEnrollInput), 201), { body: AgentEnroll })
  .get("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery })
  .delete("/agent/self", ({ request }) => handleAgentUnregister(request, new URL(request.url)), { query: AgentQuery })
  .get("/me", ({ relayUser }) => ({ user: relayUser!, workspaces: userWorkspaces(relayUser!.id) }))
  .get("/passkeys", ({ request, relayUser }) => listPasskeys(request, relayUser!).then(passkeys => ({ passkeys })))
  .post("/passkeys/options", ({ request, relayUser }) => addPasskeyOptions(request, relayUser!), { body: t.Optional(t.Object({})) })
  .post("/passkeys/verify", async ({ request, relayUser, body }) => {
    await verifyAddedPasskey(request, relayUser!, body.ceremonyId, body.response as RegistrationResponseJSON);
    return json({ ok: true }, 201);
  }, { body: WebAuthnVerify })
  .delete("/passkeys/:id", async ({ request, relayUser, params }) => {
    await deletePasskey(request, relayUser!, params.id); return { ok: true };
  }, { params: IdParams })
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
  .get("/workspaces/:workspaceId/activity", ({ relayUser, params }) => ({ events: workspaceActivity(relayUser!, params.workspaceId) }), {
    params: WorkspaceParams,
  })
  .post("/workspaces/:workspaceId/invites", ({ relayUser, params, body }) => json(createInvite(relayUser!, params.workspaceId, body.role), 201), {
    params: WorkspaceParams, body: t.Object({ role: t.Optional(t.Union([t.Literal("member"), t.Literal("viewer")])) }),
  })
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
  .get("/devices/:deviceId/processes", ({ relayUser, params }) => ({ processes: listProcesses(relayUser!.id, params.deviceId) }), {
    params: DeviceParams,
  })
  .get("/processes/:id", ({ relayUser, params }) => ({ process: getProcess(relayUser!.id, params.id) }), { params: IdParams });
