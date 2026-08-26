import { MAX_SSH_KEYS_PER_USER } from "./config";
import { id, now, q } from "./db";
import { controlProof, verifyClientSignature } from "./control-auth";
import { HttpError } from "./errors";

export type SshKeyView = {
  id: string; name: string; algorithm: string; public_key: string; created_at: number; last_used: number | null;
};

export function normalizeSshPublicKey(value: unknown) {
  const input = String(value || "").trim();
  if (!input || input.length > 16_384) throw new HttpError(400, "invalid SSH public key");
  const [algorithm, data] = input.split(/\s+/, 3);
  if (!algorithm || !data || !/^(ssh-|ecdsa-|sk-)/.test(algorithm) || !/^[A-Za-z0-9+/=]+$/.test(data)) {
    throw new HttpError(400, "invalid SSH public key");
  }
  try { if (Buffer.from(data, "base64").length < 16) throw new Error(); }
  catch { throw new HttpError(400, "invalid SSH public key"); }
  return { input, algorithm, data, publicKey: `${algorithm} ${data}` };
}

export function listSshKeys(userId: string) {
  return q<SshKeyView>("SELECT id,name,algorithm,public_key,created_at,last_used FROM ssh_keys WHERE user_id=? ORDER BY created_at DESC").all(userId);
}

export async function createSshKey(userId: string, input: {
  name?: unknown; publicKey?: unknown; clientId?: unknown; signature?: unknown;
}) {
  const count = q<{ count: number }>("SELECT count(*) count FROM ssh_keys WHERE user_id=?").get(userId)?.count || 0;
  if (count >= MAX_SSH_KEYS_PER_USER) throw new HttpError(409, `SSH key limit reached (${MAX_SSH_KEYS_PER_USER})`);
  const key = normalizeSshPublicKey(input.publicKey), clientId = String(input.clientId || "").trim();
  const signature = String(input.signature || "").trim(), name = String(input.name || "SSH key").trim().slice(0, 80) || "SSH key";
  if (!clientId || !controlProof(userId, clientId)) throw new HttpError(401, "active passkey-backed control authorization required");
  const payload = `rc-ssh-key-v1\n${clientId}\n${key.input}`;
  if (!await verifyClientSignature(userId, clientId, payload, signature)) throw new HttpError(401, "invalid SSH key authorization");
  const keyId = id(), created = now();
  try {
    q(`INSERT INTO ssh_keys(id,user_id,name,algorithm,key_data,public_key,control_client_id,created_at)
      VALUES(?,?,?,?,?,?,?,?)`).run(keyId, userId, name, key.algorithm, key.data, key.publicKey, clientId, created);
  } catch (error) {
    if (String(error).includes("UNIQUE")) throw new HttpError(409, "SSH key already registered");
    throw error;
  }
  return { id: keyId, name, algorithm: key.algorithm, publicKey: key.publicKey, createdAt: created };
}

export function deleteSshKey(userId: string, keyId: string) {
  if (!q("DELETE FROM ssh_keys WHERE id=? AND user_id=?").run(keyId, userId).changes) throw new HttpError(404, "SSH key not found");
  return { ok: true as const };
}

export function sshKeyAuthorization(algorithm: string, keyData: string) {
  return q<any>(`SELECT k.*,u.name user_name,c.grant,c.credential_id,c.assertion,c.expires_at control_expires
    FROM ssh_keys k JOIN users u ON u.id=k.user_id JOIN control_clients c ON c.id=k.control_client_id
    WHERE k.algorithm=? AND k.key_data=? AND (c.expires_at=0 OR c.expires_at>?)`).get(algorithm, keyData, now()) || null;
}

export function sshPrincipalForDevice(keyId: string, deviceId: string) {
  return q<any>(`SELECT k.id key_id,k.user_id,d.id device_id,wm.role,c.grant,c.credential_id,c.assertion,c.expires_at control_expires
    FROM ssh_keys k JOIN control_clients c ON c.id=k.control_client_id
    JOIN devices d ON d.id=? JOIN workspace_members wm ON wm.workspace_id=d.workspace_id AND wm.user_id=k.user_id
    WHERE k.id=? AND wm.role IN ('owner','operator') AND (c.expires_at=0 OR c.expires_at>?)`).get(deviceId, keyId, now()) || null;
}

export function touchSshKey(keyId: string) {
  q("UPDATE ssh_keys SET last_used=? WHERE id=?").run(now(), keyId);
}
