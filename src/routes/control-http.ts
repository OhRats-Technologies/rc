import { Elysia, t } from "elysia";
import { apiKeyGrant } from "../account";
import { auth } from "../auth";
import {
  closeControlSession, reportControlTransport, requestControlChallenge, requestControlOpen, requestControlWebRTC,
  syncWorkspaceAuthority,
} from "../control-signaling";
import { HttpError } from "../errors";
import { checkOrigin, fail } from "../http-utils";

const CandidateSummary = t.Object({
  host: t.Integer({ minimum: 0, maximum: 64 }), srflx: t.Integer({ minimum: 0, maximum: 64 }),
  relay: t.Integer({ minimum: 0, maximum: 64 }), udp: t.Integer({ minimum: 0, maximum: 64 }), tcp: t.Integer({ minimum: 0, maximum: 64 }),
});
const SessionParams = t.Object({ sessionId: t.String({ minLength: 1, maxLength: 100 }) });

export const controlHttpRoutes = new Elysia({ name: "rc.control.http", prefix: "/api/v1" })
  .onRequest(async ({ request }) => {
    if (request.headers.has("authorization") && request.headers.has("x-rc-key-id")) {
      return fail("multiple authentication credentials are not allowed", 400);
    }
    if (request.headers.has("x-rc-key-id")) await apiKeyGrant(request);
  })
  .derive(async ({ request }) => ({ rcUser: await auth(request), rcApiGrant: await apiKeyGrant(request) }))
  .onBeforeHandle(({ request, set, rcUser, rcApiGrant }) => {
    set.headers["cache-control"] = "no-store";
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    if (!rcUser) return fail("authentication required", 401);
    if (rcApiGrant && !rcApiGrant.scopes.some(scope => scope === "execute" || scope === "manage-devices")) {
      return fail("API key requires execute or manage-devices scope", 403);
    }
  })
  .onError(({ error, code, status }) => {
    if (error instanceof HttpError) return status(error.status, { error: error.message });
    if (code === "VALIDATION") return status(400, { error: "invalid request" });
  })
  .post("/control/challenge", ({ rcUser, body }) => requestControlChallenge(rcUser!.id, body.deviceId), {
    body: t.Object({ deviceId: t.String({ minLength: 1, maxLength: 100 }) }), detail: { hide: true },
  })
  .post("/control/open", ({ rcUser, rcApiGrant, body }) => requestControlOpen(rcUser!.id, body, rcApiGrant?.keyId || null), {
    body: t.Object({
      deviceId: t.String({ minLength: 1, maxLength: 100 }), challenge: t.String({ minLength: 1, maxLength: 512 }),
      clientId: t.String({ minLength: 1, maxLength: 100 }), publicKey: t.String({ minLength: 1, maxLength: 512 }),
      signature: t.String({ minLength: 1, maxLength: 512 }),
    }), detail: { hide: true },
  })
  .post("/control/:sessionId/webrtc", ({ rcUser, params, body }) => requestControlWebRTC(rcUser!.id, params.sessionId, body), {
    params: SessionParams,
    body: t.Object({ deviceId: t.String({ minLength: 1, maxLength: 100 }), sdp: t.String({ minLength: 1, maxLength: 131072 }) }),
    detail: { hide: true },
  })
  .post("/control/:sessionId/transport", ({ rcUser, params, body }) => reportControlTransport(rcUser!.id, params.sessionId, body), {
    params: SessionParams,
    body: t.Object({
      phase: t.Optional(t.Union([t.Literal("connecting"), t.Literal("connected"), t.Literal("failed")])),
      reason: t.Optional(t.String({ maxLength: 200 })), iceState: t.Optional(t.String({ maxLength: 40 })),
      connectionState: t.Optional(t.String({ maxLength: 40 })), localCandidates: t.Optional(CandidateSummary),
      remoteCandidates: t.Optional(CandidateSummary), selected: t.Optional(t.Object({
        localType: t.Optional(t.String({ maxLength: 20 })), remoteType: t.Optional(t.String({ maxLength: 20 })),
        protocol: t.Optional(t.String({ maxLength: 20 })),
      })),
    }), detail: { hide: true },
  })
  .delete("/control/:sessionId", ({ rcUser, params }) => closeControlSession(rcUser!.id, params.sessionId), {
    params: SessionParams, detail: { hide: true },
  })
  .post("/workspaces/:workspaceId/authority/sync", ({ rcUser, params, body }) =>
    syncWorkspaceAuthority(rcUser!.id, params.workspaceId, body.clientId, body.transitions), {
    params: t.Object({ workspaceId: t.String({ minLength: 1, maxLength: 100 }) }),
    body: t.Object({
      clientId: t.String({ minLength: 1, maxLength: 100 }),
      transitions: t.Array(t.Object({
        fromHash: t.String({ minLength: 64, maxLength: 64 }), generation: t.Integer({ minimum: 0 }), signature: t.String({ minLength: 1, maxLength: 512 }),
      }), { minItems: 1, maxItems: 100 }),
    }), detail: { hide: true },
  });
