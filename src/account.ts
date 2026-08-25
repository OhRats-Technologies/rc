import { MAX_API_KEYS_PER_USER } from "./config";
import { id, now, opaque, q, sha } from "./db";
import { HttpError } from "./errors";

export const API_SCOPES = ["read", "execute", "manage-devices", "manage-workspaces"] as const;
export type ApiScope = typeof API_SCOPES[number];
export type ApiTokenView = { id: string; name: string; scopes: ApiScope[]; created_at: number; last_used: number | null };

function parseScopes(value: string) {
  try {
    const parsed = JSON.parse(value) as unknown[];
    return API_SCOPES.filter(scope => parsed.includes(scope));
  } catch { return [] as ApiScope[]; }
}

export function normalizeApiScopes(value: unknown): ApiScope[] {
  if (!Array.isArray(value)) return ["read", "execute"];
  const scopes = API_SCOPES.filter(scope => value.includes(scope));
  return scopes.length ? scopes : ["read"];
}

export function listApiTokens(userId: string) {
  const rows = q<Omit<ApiTokenView, "scopes"> & { scopes: string }>(
    "SELECT id,name,scopes,created_at,last_used FROM api_tokens WHERE user_id=? ORDER BY created_at DESC"
  ).all(userId);
  return rows.map(row => ({ ...row, scopes: parseScopes(row.scopes) }));
}

export function createApiToken(userId: string, value: unknown, requestedScopes?: unknown) {
  const count = q<{ count: number }>("SELECT count(*) count FROM api_tokens WHERE user_id=?").get(userId)?.count || 0;
  if (count >= MAX_API_KEYS_PER_USER) throw new HttpError(409, `API key limit reached (${MAX_API_KEYS_PER_USER})`);
  const name = String(value || "API token").trim().slice(0, 80) || "API token";
  const scopes = normalizeApiScopes(requestedScopes), token = opaque("rc_api"), tokenId = id();
  q("INSERT INTO api_tokens(id,user_id,name,token_hash,scopes,created_at) VALUES(?,?,?,?,?,?)")
    .run(tokenId, userId, name, sha(token), JSON.stringify(scopes), now());
  return { id: tokenId, token, scopes };
}

export function deleteApiToken(userId: string, tokenId: string) {
  return q("DELETE FROM api_tokens WHERE id=? AND user_id=?").run(tokenId, userId).changes > 0;
}

export function apiTokenGrant(token: string) {
  const row = q<any>(`SELECT u.id,u.name,a.id token_id,a.scopes FROM api_tokens a JOIN users u ON u.id=a.user_id
    WHERE a.token_hash=?`).get(sha(token));
  if (!row) return null;
  q("UPDATE api_tokens SET last_used=? WHERE id=?").run(now(), row.token_id);
  return { user: { id: row.id as string, name: row.name as string }, scopes: parseScopes(String(row.scopes || "[]")) };
}

export function requiredApiScope(method: string, path: string): ApiScope | "human" | null {
  if (path.startsWith("/api/v1/tokens") || path.startsWith("/api/v1/passkeys")) return "human";
  if (method === "GET") return "read";
  if (/^\/api\/v1\/devices\/[^/]+\/processes$/.test(path) || /^\/api\/v1\/actions\/[^/]+\/run$/.test(path)) return "execute";
  if (path.includes("/enrollments") || path.startsWith("/api/v1/devices/")) return "manage-devices";
  if (path.startsWith("/api/v1/workspaces") || path.startsWith("/api/v1/actions")) return "manage-workspaces";
  return null;
}
