import { MAX_API_KEYS_PER_USER } from "./config";
import { id, now, q, sha } from "./db";
import { base64urlToBytes } from "./encoding";
import { HttpError } from "./errors";
import { API_DEFAULT_LIFETIME, authLifetime, expiresAt } from "./lifetimes";

export const API_SCOPES = ["read", "execute", "manage-devices", "manage-workspaces"] as const;
export type ApiScope = typeof API_SCOPES[number];
export type ApiTokenView = { id: string; name: string; public_key: string; scopes: ApiScope[]; created_at: number; expires_at: number; last_used: number | null };

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
    "SELECT id,name,public_key,scopes,created_at,expires_at,last_used FROM api_tokens WHERE user_id=? ORDER BY created_at DESC"
  ).all(userId);
  return rows.map(row => ({ ...row, scopes: parseScopes(row.scopes) }));
}

function validPublicKey(value: string) {
  try { return base64urlToBytes(value).length === 32; } catch { return false; }
}

export function createApiToken(userId: string, value: unknown, requestedScopes: unknown, publicKeyValue: unknown, lifetimeValue?: unknown) {
  const count = q<{ count: number }>("SELECT count(*) count FROM api_tokens WHERE user_id=?").get(userId)?.count || 0;
  if (count >= MAX_API_KEYS_PER_USER) throw new HttpError(409, `API key limit reached (${MAX_API_KEYS_PER_USER})`);
  const name = String(value || "API key").trim().slice(0, 80) || "API key";
  const scopes = normalizeApiScopes(requestedScopes), publicKey = String(publicKeyValue || "").trim(), tokenId = id(), t = now();
  const lifetime = authLifetime(lifetimeValue, API_DEFAULT_LIFETIME), expiration = expiresAt(lifetime, t);
  if (!validPublicKey(publicKey)) throw new HttpError(400, "invalid API signing key");
  q("INSERT INTO api_tokens(id,user_id,name,token_hash,public_key,scopes,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)")
    .run(tokenId, userId, name, `pop:${tokenId}`, publicKey, JSON.stringify(scopes), t, expiration);
  return { id: tokenId, publicKey, scopes, expiresAt: expiration };
}

export function deleteApiToken(userId: string, tokenId: string) {
  return q("DELETE FROM api_tokens WHERE id=? AND user_id=?").run(tokenId, userId).changes > 0;
}

type ApiKeyGrant = { user: { id: string; name: string }; scopes: ApiScope[]; keyId: string };
const requestGrants = new WeakMap<Request, ApiKeyGrant | null>();

async function bodyHash(request: Request) {
  const bytes = new Uint8Array(await request.clone().arrayBuffer());
  return new Bun.CryptoHasher("sha256").update(bytes).digest("hex");
}

export async function apiKeyGrant(request: Request): Promise<ApiKeyGrant | null> {
  if (requestGrants.has(request)) return requestGrants.get(request) || null;
  const keyId = request.headers.get("x-rc-key-id") || "", timestamp = request.headers.get("x-rc-timestamp") || "";
  const nonce = request.headers.get("x-rc-nonce") || "", signature = request.headers.get("x-rc-signature") || "";
  const seconds = Number(timestamp), t = now();
  if (!keyId || !nonce || !signature || !Number.isFinite(seconds) || Math.abs(t / 1000 - seconds) > 60) {
    requestGrants.set(request, null); return null;
  }
  const row = q<any>(`SELECT u.id,u.name,a.id token_id,a.public_key,a.scopes FROM api_tokens a
    JOIN users u ON u.id=a.user_id WHERE a.id=? AND a.public_key<>'' AND (a.expires_at=0 OR a.expires_at>?)`).get(keyId, t);
  if (!row) { requestGrants.set(request, null); return null; }
  q("DELETE FROM api_request_nonces WHERE expires_at<=?").run(t);
  if (q("SELECT 1 FROM api_request_nonces WHERE token_id=? AND nonce_hash=?").get(keyId, sha(nonce))) {
    requestGrants.set(request, null); return null;
  }
  try {
    const url = new URL(request.url), digest = await bodyHash(request);
    const payload = `rc-api-v1\n${keyId}\n${timestamp}\n${nonce}\n${request.method}\n${url.pathname}${url.search}\n${digest}`;
    const key = await crypto.subtle.importKey("raw", base64urlToBytes(row.public_key), { name: "Ed25519" }, false, ["verify"]);
    if (!await crypto.subtle.verify("Ed25519", key, base64urlToBytes(signature), new TextEncoder().encode(payload))) {
      requestGrants.set(request, null); return null;
    }
  } catch { requestGrants.set(request, null); return null; }
  try { q("INSERT INTO api_request_nonces(token_id,nonce_hash,expires_at) VALUES(?,?,?)").run(keyId, sha(nonce), t + 2 * 60_000); }
  catch { requestGrants.set(request, null); return null; }
  q("UPDATE api_tokens SET last_used=? WHERE id=?").run(t, keyId);
  const grant = { user: { id: row.id as string, name: row.name as string }, scopes: parseScopes(String(row.scopes || "[]")), keyId };
  requestGrants.set(request, grant); return grant;
}

export function requiredApiScope(method: string, path: string): ApiScope | "human" | null {
  if (path.startsWith("/api/v1/tokens") || path.startsWith("/api/v1/passkeys")) return "human";
  if (method === "GET") return "read";
  if (/^\/api\/v1\/devices\/[^/]+\/processes$/.test(path) || /^\/api\/v1\/actions\/[^/]+\/run$/.test(path)) return "execute";
  if (path.includes("/enrollments") || path.startsWith("/api/v1/devices/")) return "manage-devices";
  if (path.startsWith("/api/v1/workspaces") || path.startsWith("/api/v1/actions")) return "manage-workspaces";
  return null;
}
