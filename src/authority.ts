import { q } from "./db";

export type AuthorityCredential = { id: string; publicKey: string };
export type AuthorityMember = {
  userId: string; role: "owner" | "operator" | "viewer"; credentials: AuthorityCredential[];
};
export type AuthorityApiKey = { id: string; userId: string; publicKey: string; scopes: string[]; expiresAt: number };
export type AuthorityMcpGrant = { id: string; userId: string; hash: string };
export type AuthoritySnapshot = {
  v: 1; workspaceId: string; members: AuthorityMember[]; apiKeys: AuthorityApiKey[]; mcpGrants?: AuthorityMcpGrant[];
};

export function authoritySnapshot(workspaceId: string): AuthoritySnapshot {
  const members = q<any>(`SELECT wm.user_id userId,wm.role,p.credential_id credentialId,p.public_key publicKey
    FROM workspace_members wm LEFT JOIN passkeys p ON p.user_id=wm.user_id
    WHERE wm.workspace_id=? ORDER BY wm.user_id,p.credential_id`).all(workspaceId);
  const grouped = new Map<string, AuthorityMember>();
  for (const row of members) {
    let member = grouped.get(row.userId);
    if (!member) {
      member = { userId: row.userId, role: row.role, credentials: [] };
      grouped.set(row.userId, member);
    }
    if (row.credentialId && row.publicKey) member.credentials.push({ id: row.credentialId, publicKey: row.publicKey });
  }
  const apiKeys = q<any>(`SELECT a.id,a.user_id userId,a.public_key publicKey,a.scopes,a.expires_at expiresAt
    FROM api_tokens a JOIN workspace_members wm ON wm.user_id=a.user_id
    WHERE wm.workspace_id=? AND a.public_key<>'' AND (a.expires_at=0 OR a.expires_at>?) ORDER BY a.id`).all(workspaceId, Date.now()).map(row => ({
      id: String(row.id), userId: String(row.userId), publicKey: String(row.publicKey),
      scopes: JSON.parse(String(row.scopes || "[]")) as string[], expiresAt: Number(row.expiresAt || 0),
    }));
  const mcpGrants = q<{ id: string; user_id: string; grant: string }>(`SELECT DISTINCT g.id,g.user_id,g.grant FROM mcp_grants g
    JOIN json_each(json_extract(g.grant,'$.deviceIds')) granted JOIN devices d ON d.id=granted.value
    WHERE d.workspace_id=? AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?) AND EXISTS (
      SELECT 1 FROM json_each(json_extract(g.grant,'$.scopes')) scope WHERE scope.value IN ('mcp:actions','mcp:terminal')
    ) ORDER BY g.id`).all(workspaceId, Date.now())
    .map(row => ({ id: row.id, userId: row.user_id, hash: new Bun.CryptoHasher("sha256").update(row.grant).digest("hex") }));
  return { v: 1, workspaceId, members: [...grouped.values()], apiKeys, ...(mcpGrants.length ? { mcpGrants } : {}) };
}

export function canonicalAuthority(snapshot: AuthoritySnapshot) {
  return JSON.stringify(snapshot);
}

export function authorityHash(workspaceId: string) {
  const snapshot = canonicalAuthority(authoritySnapshot(workspaceId));
  return { snapshot, hash: new Bun.CryptoHasher("sha256").update(snapshot).digest("hex") };
}

