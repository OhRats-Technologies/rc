import { deviceRole, logEvent, type User } from "../core";
import { id, now, q } from "../db";
import { freshControlProof, verifyClientSignature } from "../control-auth";
import { MCP_GRANT_TTL } from "../config";
import { HttpError } from "../errors";
import { MCP_SCOPES, type McpGrantPayload, type McpGrantRecord, type McpScope } from "./types";

export function actionHash(command: string, cwd: string | null) {
  return new Bun.CryptoHasher("sha256").update(`${command}\n${cwd || ""}`).digest("hex");
}

export function grantWorkspaceIds(payload: McpGrantPayload) {
  if (!payload.deviceIds.length) return [];
  return q<{ workspace_id: string }>(`SELECT DISTINCT workspace_id FROM devices WHERE id IN (${payload.deviceIds.map(() => "?").join(",")}) ORDER BY workspace_id`)
    .all(...payload.deviceIds).map(row => row.workspace_id);
}

function normalizedScopes(value: unknown): McpScope[] {
  const requested = Array.isArray(value) ? value.map(String) : [];
  const scopes = MCP_SCOPES.filter(scope => requested.includes(scope));
  return scopes.length ? scopes : ["mcp:observe"];
}

function allowedDeviceIds(user: User, values: unknown, scopes: McpScope[]) {
  const requested = [...new Set(Array.isArray(values) ? values.map(String) : [])].slice(0, 100);
  if (!requested.length) throw new HttpError(400, "select at least one device");
  for (const deviceId of requested) {
    const role = deviceRole(user.id, deviceId);
    if (!role) throw new HttpError(403, "device is not available to this account");
    if (scopes.some(scope => scope !== "mcp:observe") && role !== "owner") {
      throw new HttpError(403, "Actions and Terminal require Owner access on every selected device");
    }
  }
  return requested.sort();
}

export function prepareGrant(user: User, clientId: string, clientName: string, deviceIdsValue: unknown, scopesValue: unknown) {
  const scopes = normalizedScopes(scopesValue), deviceIds = allowedDeviceIds(user, deviceIdsValue, scopes), issuedAt = now();
  const placeholders = deviceIds.map(() => "?").join(",");
  const workspaces = q<{ workspace_id: string }>(`SELECT DISTINCT workspace_id FROM devices WHERE id IN (${placeholders})`)
    .all(...deviceIds).map(row => row.workspace_id);
  const actionPlaceholders = workspaces.map(() => "?").join(",");
  const actions = scopes.includes("mcp:actions") && workspaces.length
    ? q<{ id: string; command: string; cwd: string | null }>(`SELECT id,command,cwd FROM actions WHERE workspace_id IN (${actionPlaceholders}) ORDER BY id`)
      .all(...workspaces).map(action => ({ id: action.id, hash: actionHash(action.command, action.cwd) })) : [];
  if (actions.length > 400) throw new HttpError(409, "too many saved Actions for one MCP grant");
  const payload: McpGrantPayload = {
    v: 1, id: id(), userId: user.id, clientId, clientName, deviceIds, scopes, actions,
    issuedAt, expiresAt: issuedAt + MCP_GRANT_TTL,
  };
  return JSON.stringify(payload);
}

export function grantSignaturePayload(grant: string) {
  const digest = new Bun.CryptoHasher("sha256").update(grant).digest("hex");
  return `rc-mcp-grant-v1\n${digest}`;
}

export async function persistGrant(user: User, grant: string, controlClientId: string, signature: string) {
  const payload = JSON.parse(grant) as McpGrantPayload;
  if (payload.v !== 1 || payload.userId !== user.id || payload.expiresAt <= now()) throw new HttpError(400, "invalid MCP grant");
  if (!await verifyClientSignature(user.id, controlClientId, grantSignaturePayload(grant), signature)) {
    throw new HttpError(401, "invalid MCP grant signature");
  }
  const proof = freshControlProof(user.id, controlClientId); if (!proof) throw new HttpError(401, "fresh passkey authorization required");
  q(`INSERT INTO mcp_grants(id,user_id,client_id,name,grant,control_client_id,grant_signature,credential_id,control_grant,control_assertion,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`).run(payload.id, user.id, payload.clientId, payload.clientName, grant, controlClientId, signature,
    proof.credentialId, proof.grant, proof.assertion, payload.issuedAt, payload.expiresAt);
  for (const deviceId of payload.deviceIds) {
    const workspaceId = q<{ workspace_id: string }>("SELECT workspace_id FROM devices WHERE id=?").get(deviceId)?.workspace_id || null;
    logEvent("mcp.granted", workspaceId, user.id, deviceId, { grantId: payload.id, client: payload.clientName, scopes: payload.scopes });
  }
  return payload;
}

export function parseGrant(record: McpGrantRecord) { return JSON.parse(record.grant) as McpGrantPayload; }

export function listMcpGrants(userId: string) {
  return q<McpGrantRecord>("SELECT * FROM mcp_grants WHERE user_id=? AND revoked_at IS NULL AND expires_at>? ORDER BY created_at DESC")
    .all(userId, now()).map(record => ({ record, payload: parseGrant(record) }));
}

export function revokeMcpGrant(userId: string, grantId: string) {
  const record = q<McpGrantRecord>("SELECT * FROM mcp_grants WHERE id=? AND user_id=?").get(grantId, userId);
  if (!record) return null;
  q("UPDATE mcp_grants SET revoked_at=coalesce(revoked_at,?) WHERE id=?").run(now(), grantId);
  q("DELETE FROM mcp_access_tokens WHERE grant_id=?").run(grantId);
  q("DELETE FROM mcp_refresh_tokens WHERE grant_id=?").run(grantId);
  return grantWorkspaceIds(parseGrant(record));
}
