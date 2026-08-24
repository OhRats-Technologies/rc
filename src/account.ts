import { id, now, opaque, q, sha } from "./db";

export type ApiTokenView = { id: string; name: string; created_at: number; last_used: number | null };

export function listApiTokens(userId: string) {
  return q<ApiTokenView>("SELECT id,name,created_at,last_used FROM api_tokens WHERE user_id=? ORDER BY created_at DESC").all(userId);
}

export function createApiToken(userId: string, value: unknown) {
  const name = String(value || "API token").trim().slice(0, 80) || "API token";
  const token = opaque("rly"), tokenId = id();
  q("INSERT INTO api_tokens(id,user_id,name,token_hash,created_at) VALUES(?,?,?,?,?)")
    .run(tokenId, userId, name, sha(token), now());
  return { id: tokenId, token };
}

export function deleteApiToken(userId: string, tokenId: string) {
  return q("DELETE FROM api_tokens WHERE id=? AND user_id=?").run(tokenId, userId).changes > 0;
}
