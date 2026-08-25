import { Elysia } from "elysia";
import { cookieUser, logout } from "../auth";
import { PUBLIC_URL } from "../config";
import { deviceRole } from "../core";
import { listDevices } from "../devices";
import { HttpError } from "../errors";
import { checkOrigin, fail, json, sessionCookie } from "../http-utils";
import { listMcpGrants, revokeMcpGrant } from "../mcp/grants";
import {
  approveOAuthGrant, createOAuthRequest, denyOAuthRequest, exchangeOAuthToken, MCP_RESOURCE, MCP_RESOURCE_METADATA,
  prepareOAuthGrant, registerMcpClient, restartOAuthRequest,
} from "../mcp/oauth";
import { MCP_SCOPES } from "../mcp/types";
import { pageContext } from "../page-context";
import { mcpAuthorizePage, mcpConnectionsPage } from "../../web/server/pages/mcp";

function oauthMetadata() {
  return {
    issuer: PUBLIC_URL, authorization_endpoint: `${PUBLIC_URL}/oauth/authorize`, token_endpoint: `${PUBLIC_URL}/oauth/token`,
    registration_endpoint: `${PUBLIC_URL}/oauth/register`, response_types_supported: ["code"],
    grant_types_supported: ["authorization_code", "refresh_token"], token_endpoint_auth_methods_supported: ["none"],
    code_challenge_methods_supported: ["S256"], scopes_supported: [...MCP_SCOPES], authorization_response_iss_parameter_supported: true,
  };
}

function protectedMetadata() {
  return { resource: MCP_RESOURCE, authorization_servers: [PUBLIC_URL], scopes_supported: ["mcp:observe", "mcp:actions"], bearer_methods_supported: ["header"] };
}

function oauthError(error: unknown) {
  const message = error instanceof Error ? error.message : "OAuth request failed";
  return json({ error: error instanceof HttpError && error.status === 401 ? "invalid_grant" : "invalid_request", error_description: message },
    error instanceof HttpError ? error.status : 400);
}

export const oauthRoutes = new Elysia({ name: "rc.oauth", detail: { hide: true } })
  .get("/.well-known/oauth-protected-resource", () => protectedMetadata())
  .get("/.well-known/oauth-protected-resource/mcp", () => protectedMetadata())
  .get("/.well-known/oauth-authorization-server", () => oauthMetadata())
  .post("/oauth/register", async ({ request }) => {
    try { return json(registerMcpClient(await request.json() as Record<string, unknown>), 201); } catch (error) { return oauthError(error); }
  })
  .get("/oauth/authorize", async ({ request }) => {
    const user = await cookieUser(request);
    if (!user) {
      const url = new URL(request.url), next = `${url.pathname}${url.search}`;
      return Response.redirect(`/?next=${encodeURIComponent(next)}`, 303);
    }
    try {
      const authorization = createOAuthRequest(user, new URL(request.url));
      const devices = listDevices(user).map(device => ({ id: device.id, name: device.name, workspace_name: device.workspace_name,
        role: deviceRole(user.id, device.id) || "viewer", online: device.online }));
      return mcpAuthorizePage(user, authorization.requestId, authorization.clientName, authorization.redirectUri, authorization.requestedScopes, devices);
    } catch (error) { return new Response(error instanceof Error ? error.message : "invalid OAuth request", { status: 400, headers: { "cache-control": "no-store" } }); }
  })
  .post("/oauth/authorize/prepare", async ({ request }) => {
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    try {
      const body = await request.json() as any;
      return json(prepareOAuthGrant(user, String(body.requestId || ""), body.deviceIds, body.scopes));
    } catch (error) { return error instanceof HttpError ? fail(error.message, error.status) : fail("authorization failed", 400); }
  })
  .post("/oauth/authorize/approve", async ({ request }) => {
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    try {
      const body = await request.json() as any;
      return json(await approveOAuthGrant(user, String(body.requestId || ""), String(body.controlClientId || ""), String(body.signature || "")));
    } catch (error) { return error instanceof HttpError ? fail(error.message, error.status) : fail("authorization failed", 400); }
  })
  .post("/oauth/authorize/cancel", async ({ request }) => {
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    try {
      const body = await request.json() as any;
      return json({ redirect: denyOAuthRequest(user.id, String(body.requestId || "")) });
    } catch (error) { return error instanceof HttpError ? fail(error.message, error.status) : fail("authorization failed", 400); }
  })
  .post("/oauth/authorize/switch-account", async ({ request }) => {
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    try {
      const body = await request.json() as any, next = restartOAuthRequest(user.id, String(body.requestId || ""));
      logout(request);
      return new Response(JSON.stringify({ redirect: `/?next=${encodeURIComponent(next)}` }), {
        headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", "set-cookie": sessionCookie("", 0) },
      });
    } catch (error) { return error instanceof HttpError ? fail(error.message, error.status) : fail("authorization failed", 400); }
  })
  .post("/oauth/token", async ({ request }) => {
    try { return json(exchangeOAuthToken(new URLSearchParams(await request.text()))); } catch (error) { return oauthError(error); }
  })
  .delete("/oauth/grants/:id", async ({ request, params }) => {
    if (!checkOrigin(request)) return fail("invalid origin", 403);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    const workspaceIds = revokeMcpGrant(user.id, params.id); if (!workspaceIds) return fail("MCP grant not found", 404);
    return json({ ok: true, workspaceIds });
  })
  .get("/integrations/mcp", async ({ request }) => {
    const context = await pageContext(request); if (!context) return Response.redirect("/", 303);
    return mcpConnectionsPage(context.user, context.workspaces, context.sidebar, MCP_RESOURCE, listMcpGrants(context.user.id));
  })
  ;
