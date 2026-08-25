import { generateAuthenticationOptions, verifyAuthenticationResponse, type AuthenticationResponseJSON } from "@simplewebauthn/server";
import { CONTROL_GRANT_TTL, PUBLIC_URL, RP_ID } from "./config";
import type { User } from "./core";
import { id, now, q } from "./db";
import { base64ToBytes, base64urlToBytes, bytesToBase64url } from "./encoding";
import { HttpError } from "./errors";

export type ControlGrant = {
  v: 1; clientId: string; userId: string; signingPublicKey: string; issuedAt: number; expiresAt: number;
};
export type ControlProof = { grant: string; credentialId: string; assertion: string };

export function canonicalGrant(grant: ControlGrant) { return JSON.stringify(grant); }

export function controlGrantChallenge(grant: string) {
  const bytes = new TextEncoder().encode(`rc-control-grant-v1\n${grant}`);
  const digest = new Bun.CryptoHasher("sha256").update(bytes).digest();
  return bytesToBase64url(new Uint8Array(digest));
}

function passkeyDescriptors(userId: string) {
  return q<{ credential_id: string; transports: string }>("SELECT credential_id,transports FROM passkeys WHERE user_id=?")
    .all(userId).map(row => ({ id: row.credential_id, transports: JSON.parse(row.transports || "[]") }));
}

function validSigningKey(value: string) {
  try { return base64urlToBytes(value).length === 32; } catch { return false; }
}

export async function controlAuthorizationOptions(user: User, input: { clientId?: unknown; signingPublicKey?: unknown }) {
  const clientId = String(input.clientId || "").trim(), signingPublicKey = String(input.signingPublicKey || "").trim();
  if (!clientId || clientId.length > 100 || !validSigningKey(signingPublicKey)) throw new HttpError(400, "invalid control client key");
  const issuedAt = now(), grant = canonicalGrant({
    v: 1, clientId, userId: user.id, signingPublicKey, issuedAt, expiresAt: issuedAt + CONTROL_GRANT_TTL,
  });
  const challenge = controlGrantChallenge(grant);
  const options = await generateAuthenticationOptions({
    rpID: RP_ID, userVerification: "required", challenge: base64urlToBytes(challenge), allowCredentials: passkeyDescriptors(user.id),
  });
  const authorizationId = id();
  q(`INSERT INTO control_authorizations(id,user_id,client_id,signing_public_key,grant,challenge,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?)`).run(authorizationId, user.id, clientId, signingPublicKey, grant, challenge, issuedAt, issuedAt + 5 * 60_000);
  return { authorizationId, grant, options };
}

export async function verifyControlAuthorization(user: User, authorizationId: string, response: AuthenticationResponseJSON) {
  const row = q<any>("SELECT * FROM control_authorizations WHERE id=? AND user_id=? AND expires_at>?").get(authorizationId, user.id, now());
  if (!row) throw new HttpError(410, "control authorization expired");
  q("DELETE FROM control_authorizations WHERE id=?").run(authorizationId);
  const passkey = q<any>("SELECT * FROM passkeys WHERE user_id=? AND credential_id=?").get(user.id, String(response.id || ""));
  if (!passkey) throw new HttpError(401, "unknown passkey");
  try {
    const result = await verifyAuthenticationResponse({
      response, expectedChallenge: row.challenge, expectedOrigin: PUBLIC_URL, expectedRPID: RP_ID,
      requireUserVerification: true,
      credential: { id: passkey.credential_id, publicKey: base64ToBytes(passkey.public_key),
        counter: Number(passkey.counter || 0), transports: JSON.parse(passkey.transports || "[]") },
    });
    if (!result.verified) throw new Error("not verified");
    q("UPDATE passkeys SET counter=?,last_used=? WHERE id=?").run(result.authenticationInfo.newCounter, now(), passkey.id);
  } catch { throw new HttpError(401, "passkey verification failed"); }
  const grant = JSON.parse(String(row.grant)) as ControlGrant;
  q(`INSERT INTO control_clients(id,user_id,signing_public_key,credential_id,grant,assertion,created_at,expires_at,last_used)
    VALUES(?,?,?,?,?,?,?,?,NULL)
    ON CONFLICT(id) DO UPDATE SET user_id=excluded.user_id,signing_public_key=excluded.signing_public_key,
      credential_id=excluded.credential_id,grant=excluded.grant,assertion=excluded.assertion,
      created_at=excluded.created_at,expires_at=excluded.expires_at,last_used=NULL`).run(
    grant.clientId, user.id, grant.signingPublicKey, passkey.credential_id, row.grant, JSON.stringify(response),
    grant.issuedAt, grant.expiresAt,
  );
  return { clientId: grant.clientId, expiresAt: grant.expiresAt };
}

export function controlProof(userId: string, clientId: string): ControlProof | null {
  const row = q<any>(`SELECT grant,credential_id,assertion FROM control_clients
    WHERE id=? AND user_id=? AND expires_at>?`).get(clientId, userId, now());
  if (!row) return null;
  q("UPDATE control_clients SET last_used=? WHERE id=?").run(now(), clientId);
  return { grant: String(row.grant), credentialId: String(row.credential_id), assertion: String(row.assertion) };
}

export function freshControlProof(userId: string, clientId: string, maxAgeMs = 2 * 60_000): ControlProof | null {
  const row = q<any>(`SELECT grant,credential_id,assertion FROM control_clients
    WHERE id=? AND user_id=? AND expires_at>? AND created_at>=?`).get(clientId, userId, now(), now() - maxAgeMs);
  return row ? { grant: String(row.grant), credentialId: String(row.credential_id), assertion: String(row.assertion) } : null;
}

export function controlClientStatus(userId: string, clientId: string) {
  const row = q<{ expires_at: number }>("SELECT expires_at FROM control_clients WHERE id=? AND user_id=? AND expires_at>?")
    .get(clientId, userId, now());
  return row ? { authorized: true as const, expiresAt: row.expires_at } : { authorized: false as const };
}

export async function verifyClientSignature(userId: string, clientId: string, payload: string, signature: string) {
  const row = q<any>("SELECT signing_public_key FROM control_clients WHERE id=? AND user_id=? AND expires_at>?").get(clientId, userId, now());
  if (!row) return false;
  try {
    const key = await crypto.subtle.importKey("raw", base64urlToBytes(String(row.signing_public_key)), { name: "Ed25519" }, false, ["verify"]);
    return await crypto.subtle.verify("Ed25519", key, base64urlToBytes(signature), new TextEncoder().encode(payload));
  } catch { return false; }
}
