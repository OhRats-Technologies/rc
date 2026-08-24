import { User } from "./core";
import { id, now, opaque, q, sha } from "./db";
import { body, fail, json } from "./http-utils";

export async function handleTokens(req: Request, path: string, user: User): Promise<Response | null> {
  if (path === "/api/v1/tokens" && req.method === "GET") {
    const tokens = q<any>("SELECT id,name,created_at,last_used FROM api_tokens WHERE user_id=? ORDER BY created_at DESC").all(user.id);
    return json({ tokens });
  }
  if (path === "/api/v1/tokens" && req.method === "POST") {
    const input = await body(req), name = String(input.name || "API token").trim().slice(0, 80) || "API token";
    const token = opaque("rly"), tokenId = id();
    q("INSERT INTO api_tokens(id,user_id,name,token_hash,created_at) VALUES(?,?,?,?,?)")
      .run(tokenId, user.id, name, sha(token), now());
    return json({ id: tokenId, token }, 201);
  }
  const match = path.match(/^\/api\/v1\/tokens\/([^/]+)$/);
  if (match && req.method === "DELETE") {
    const removed = q("DELETE FROM api_tokens WHERE id=? AND user_id=?").run(match[1], user.id);
    return removed.changes ? json({ ok: true }) : fail("token not found", 404);
  }
  return null;
}
