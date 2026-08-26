import type { User } from "../core";
import { MCP_ACCESS_TTL, PUBLIC_URL } from "../config";
import { id, now, opaque, q, sha } from "../db";
import { HttpError } from "../errors";
import { grantWorkspaceIds, parseGrant, persistGrant, prepareGrant } from "./grants";
import { MCP_SCOPES, type McpGrantRecord, type McpScope } from "./types";

export const MCP_RESOURCE = `${PUBLIC_URL}/mcp`;
export const MCP_RESOURCE_METADATA = `${PUBLIC_URL}/.well-known/oauth-protected-resource`;

type OAuthRequestRow = {
  id: string; user_id: string; client_id: string; redirect_uri: string; state: string; requested_scope: string;
  code_challenge: string; resource: string; prepared_grant: string | null; expires_at: number; client_name?: string;
};

function scopes(value: string): McpScope[] {
  const values = value.split(/\s+/).filter(Boolean);
  if (values.some(scope => !MCP_SCOPES.includes(scope as McpScope))) throw new HttpError(400, "unsupported MCP scope");
  return MCP_SCOPES.filter(scope => values.includes(scope));
}

function safeRedirect(value: string) {
  try {
    const url = new URL(value);
    if (url.hash || url.username || url.password) return false;
    if (url.protocol === "https:") return true;
    if (url.protocol === "http:") return ["localhost", "127.0.0.1", "::1"].includes(url.hostname);
    return /^[a-z][a-z0-9+.-]*:$/.test(url.protocol) && !["javascript:", "data:", "file:"].includes(url.protocol);
  } catch { return false; }
}

function client(clientId: string) {
  return q<{ id: string; name: string; redirect_uris: string }>("SELECT * FROM mcp_clients WHERE id=?").get(clientId) || null;
}

export function registerMcpClient(input: Record<string, unknown>) {
  const redirectUris = Array.isArray(input.redirect_uris) ? [...new Set(input.redirect_uris.map(String))].slice(0, 10) : [];
  if (!redirectUris.length || redirectUris.some(uri => !safeRedirect(uri))) throw new HttpError(400, "invalid redirect_uris");
  const applicationType = String(input.application_type || "native");
  if (!['native', 'web'].includes(applicationType)) throw new HttpError(400, "invalid application_type");
  if (applicationType === "web" && redirectUris.some(uri => new URL(uri).protocol !== "https:")) {
    throw new HttpError(400, "web MCP clients require HTTPS redirect URIs");
  }
  const authMethod = String(input.token_endpoint_auth_method || "none");
  if (authMethod !== "none") throw new HttpError(400, "only public MCP clients are supported");
  const clientId = opaque("mcp_client"), name = String(input.client_name || "MCP client").trim().slice(0, 120) || "MCP client";
  q("INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)").run(clientId, name, JSON.stringify(redirectUris), now());
  return { client_id: clientId, client_id_issued_at: Math.floor(now() / 1000), client_name: name,
    redirect_uris: redirectUris, application_type: applicationType, token_endpoint_auth_method: "none",
    grant_types: ["authorization_code", "refresh_token"], response_types: ["code"] };
}

export function createOAuthRequest(user: User, url: URL) {
  const clientId = url.searchParams.get("client_id") || "", record = client(clientId);
  if (!record) throw new HttpError(400, "unknown MCP client");
  const redirectUri = url.searchParams.get("redirect_uri") || "";
  const registered = JSON.parse(record.redirect_uris) as string[];
  if (!registered.includes(redirectUri)) throw new HttpError(400, "redirect_uri is not registered");
  if (url.searchParams.get("response_type") !== "code") throw new HttpError(400, "response_type must be code");
  if (url.searchParams.get("code_challenge_method") !== "S256") throw new HttpError(400, "PKCE S256 is required");
  const challenge = url.searchParams.get("code_challenge") || "";
  if (!/^[A-Za-z0-9_-]{43,128}$/.test(challenge)) throw new HttpError(400, "invalid PKCE code challenge");
  const resource = url.searchParams.get("resource") || "";
  if (resource !== MCP_RESOURCE) throw new HttpError(400, "resource must identify this MCP server");
  const requested = scopes(url.searchParams.get("scope") || "mcp:observe");
  const state = String(url.searchParams.get("state") || ""); if (state.length > 1024) throw new HttpError(400, "state is too long");
  const requestId = id(), t = now();
  q(`INSERT INTO mcp_oauth_requests(id,user_id,client_id,redirect_uri,state,requested_scope,code_challenge,resource,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?,?,?)`).run(requestId, user.id, clientId, redirectUri, state, requested.join(" "), challenge, resource, t, t + 10 * 60_000);
  return { requestId, clientName: record.name, redirectUri, requestedScopes: requested };
}

export function oauthRequest(userId: string, requestId: string) {
  return q<OAuthRequestRow>(`SELECT r.*,c.name client_name FROM mcp_oauth_requests r JOIN mcp_clients c ON c.id=r.client_id
    WHERE r.id=? AND r.user_id=? AND r.expires_at>?`).get(requestId, userId, now()) || null;
}

function authorizationUrl(request: OAuthRequestRow) {
  const url = new URL(`${PUBLIC_URL}/oauth/authorize`);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("client_id", request.client_id);
  url.searchParams.set("redirect_uri", request.redirect_uri);
  url.searchParams.set("scope", request.requested_scope);
  url.searchParams.set("state", request.state);
  url.searchParams.set("code_challenge", request.code_challenge);
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("resource", request.resource);
  return `${url.pathname}${url.search}`;
}

export function denyOAuthRequest(userId: string, requestId: string) {
  const request = oauthRequest(userId, requestId); if (!request) throw new HttpError(410, "MCP authorization expired");
  q("DELETE FROM mcp_oauth_requests WHERE id=?").run(request.id);
  const redirect = new URL(request.redirect_uri);
  redirect.searchParams.set("error", "access_denied");
  redirect.searchParams.set("error_description", "The user declined this MCP authorization request.");
  if (request.state) redirect.searchParams.set("state", request.state);
  redirect.searchParams.set("iss", PUBLIC_URL);
  return redirect.toString();
}

export function restartOAuthRequest(userId: string, requestId: string) {
  const request = oauthRequest(userId, requestId); if (!request) throw new HttpError(410, "MCP authorization expired");
  const next = authorizationUrl(request);
  q("DELETE FROM mcp_oauth_requests WHERE id=?").run(request.id);
  return next;
}

export function prepareOAuthGrant(user: User, requestId: string, deviceIds: unknown, requestedScopes: unknown, lifetimeValue?: unknown) {
  const request = oauthRequest(user.id, requestId); if (!request) throw new HttpError(410, "MCP authorization expired");
  const selected = Array.isArray(requestedScopes) ? requestedScopes.map(String) : [];
  if (!selected.length) throw new HttpError(400, "select at least one MCP permission");
  const allowed = scopes(request.requested_scope);
  if (selected.some(scope => !allowed.includes(scope as McpScope))) throw new HttpError(400, "scope was not requested by this MCP client");
  const grant = prepareGrant(user, request.client_id, request.client_name || "MCP client", deviceIds, selected, lifetimeValue);
  q("UPDATE mcp_oauth_requests SET prepared_grant=? WHERE id=?").run(grant, request.id);
  const digest = new Bun.CryptoHasher("sha256").update(grant).digest("hex");
  return { grant, signaturePayload: `rc-mcp-grant-v1\n${digest}` };
}

export async function approveOAuthGrant(user: User, requestId: string, controlClientId: string, signature: string) {
  const request = oauthRequest(user.id, requestId); if (!request || !request.prepared_grant) throw new HttpError(410, "MCP authorization expired");
  const payload = await persistGrant(user, request.prepared_grant, controlClientId, signature);
  const code = opaque("mcp_code");
  q("INSERT INTO mcp_codes(code_hash,grant_id,redirect_uri,code_challenge,resource,expires_at) VALUES(?,?,?,?,?,?)")
    .run(sha(code), payload.id, request.redirect_uri, request.code_challenge, request.resource, now() + 5 * 60_000);
  q("DELETE FROM mcp_oauth_requests WHERE id=?").run(request.id);
  const redirect = new URL(request.redirect_uri); redirect.searchParams.set("code", code);
  if (request.state) redirect.searchParams.set("state", request.state);
  redirect.searchParams.set("iss", PUBLIC_URL);
  return { redirect: redirect.toString(), grantId: payload.id, workspaceIds: grantWorkspaceIds(payload),
    requiresSync: payload.scopes.includes("mcp:terminal") };
}

function verifierChallenge(verifier: string) {
  const digest = new Bun.CryptoHasher("sha256").update(verifier).digest();
  return Buffer.from(digest).toString("base64url");
}

function issueTokens(grant: McpGrantRecord) {
  const access = opaque("mcp_access"), refresh = opaque("mcp_refresh"), t = now();
  const accessExpires = grant.expires_at === 0 ? t + MCP_ACCESS_TTL : Math.min(t + MCP_ACCESS_TTL, grant.expires_at);
  const refreshExpires = grant.expires_at;
  q("INSERT INTO mcp_access_tokens(token_hash,grant_id,expires_at) VALUES(?,?,?)").run(sha(access), grant.id, accessExpires);
  q("INSERT INTO mcp_refresh_tokens(token_hash,grant_id,expires_at) VALUES(?,?,?)").run(sha(refresh), grant.id, refreshExpires);
  q("UPDATE mcp_grants SET last_used=? WHERE id=?").run(t, grant.id);
  const payload = parseGrant(grant);
  return { access_token: access, token_type: "Bearer", expires_in: Math.max(1, Math.floor((accessExpires - t) / 1000)),
    refresh_token: refresh, scope: payload.scopes.join(" ") };
}

export function exchangeOAuthToken(form: URLSearchParams) {
  const grantType = form.get("grant_type") || "", clientId = form.get("client_id") || "", resource = form.get("resource") || "";
  if (resource !== MCP_RESOURCE) throw new HttpError(400, "invalid resource");
  if (grantType === "authorization_code") {
    const code = form.get("code") || "", verifier = form.get("code_verifier") || "", redirectUri = form.get("redirect_uri") || "";
    if (!/^[A-Za-z0-9._~-]{43,128}$/.test(verifier)) throw new HttpError(400, "invalid PKCE verifier");
    const row = q<any>(`SELECT g.*,c.redirect_uri code_redirect_uri,c.code_challenge,c.resource FROM mcp_codes c JOIN mcp_grants g ON g.id=c.grant_id
      WHERE c.code_hash=? AND c.expires_at>? AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)`).get(sha(code), now(), now());
    if (!row || row.client_id !== clientId || row.code_redirect_uri !== redirectUri || row.resource !== resource || row.code_challenge !== verifierChallenge(verifier)) {
      throw new HttpError(400, "invalid authorization code");
    }
    q("DELETE FROM mcp_codes WHERE code_hash=?").run(sha(code));
    return issueTokens(row as McpGrantRecord);
  }
  if (grantType === "refresh_token") {
    const token = form.get("refresh_token") || "";
    const row = q<any>(`SELECT g.* FROM mcp_refresh_tokens r JOIN mcp_grants g ON g.id=r.grant_id
      WHERE r.token_hash=? AND (r.expires_at=0 OR r.expires_at>?) AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)`).get(sha(token), now(), now());
    if (!row || row.client_id !== clientId) throw new HttpError(400, "invalid refresh token");
    q("DELETE FROM mcp_refresh_tokens WHERE token_hash=?").run(sha(token));
    return issueTokens(row as McpGrantRecord);
  }
  throw new HttpError(400, "unsupported grant_type");
}

export function mcpAccessGrant(request: Request) {
  const token = request.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1] || "";
  if (!token) return null;
  const grant = q<McpGrantRecord>(`SELECT g.* FROM mcp_access_tokens a JOIN mcp_grants g ON g.id=a.grant_id
    WHERE a.token_hash=? AND a.expires_at>? AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)`).get(sha(token), now(), now()) || null;
  if (grant) q("UPDATE mcp_grants SET last_used=? WHERE id=?").run(now(), grant.id);
  return grant;
}

export function cleanupMcpOAuth() {
  const t = now();
  q("DELETE FROM mcp_oauth_requests WHERE expires_at<=?").run(t);
  q("DELETE FROM mcp_codes WHERE expires_at<=?").run(t);
  q("DELETE FROM mcp_access_tokens WHERE expires_at<=?").run(t);
  q("DELETE FROM mcp_refresh_tokens WHERE expires_at>0 AND expires_at<=?").run(t);
  q(`DELETE FROM mcp_clients WHERE created_at<?
    AND NOT EXISTS(SELECT 1 FROM mcp_oauth_requests r WHERE r.client_id=mcp_clients.id)
    AND NOT EXISTS(SELECT 1 FROM mcp_grants g WHERE g.client_id=mcp_clients.id)`).run(t - 24 * 60 * 60_000);
}
