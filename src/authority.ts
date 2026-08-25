import { q } from "./db";

export type AuthorityCredential = { id: string; publicKey: string };
export type AuthorityMember = {
  userId: string; role: "owner" | "operator" | "viewer"; credentials: AuthorityCredential[];
};
export type AuthorityApiKey = { id: string; userId: string; publicKey: string; scopes: string[] };
export type AuthoritySnapshot = {
  v: 1; workspaceId: string; members: AuthorityMember[]; apiKeys: AuthorityApiKey[];
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
  const apiKeys = q<any>(`SELECT a.id,a.user_id userId,a.public_key publicKey,a.scopes
    FROM api_tokens a JOIN workspace_members wm ON wm.user_id=a.user_id
    WHERE wm.workspace_id=? AND a.public_key<>'' ORDER BY a.id`).all(workspaceId).map(row => ({
      id: String(row.id), userId: String(row.userId), publicKey: String(row.publicKey),
      scopes: JSON.parse(String(row.scopes || "[]")) as string[],
    }));
  return { v: 1, workspaceId, members: [...grouped.values()], apiKeys };
}

export function canonicalAuthority(snapshot: AuthoritySnapshot) {
  return JSON.stringify(snapshot);
}

export function authorityHash(workspaceId: string) {
  const snapshot = canonicalAuthority(authoritySnapshot(workspaceId));
  return { snapshot, hash: new Bun.CryptoHasher("sha256").update(snapshot).digest("hex") };
}

