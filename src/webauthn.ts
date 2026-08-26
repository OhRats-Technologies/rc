import {
  generateAuthenticationOptions,
  generateRegistrationOptions,
  verifyRegistrationResponse,
  type RegistrationResponseJSON,
  type WebAuthnCredential,
} from "@simplewebauthn/server";
import { CEREMONY_TTL, PUBLIC_URL, RP_ID } from "./config";
import { id, now, q } from "./db";
import { bytesToBase64 } from "./encoding";

export type CeremonyKind = "setup" | "register" | "login" | "add-passkey";
export type CeremonyRow = {
  id: string; challenge: string; kind: CeremonyKind; user_id: string | null; name: string | null;
  invite_id: string | null; created_at: number; expires_at: number;
};

export function cleanName(value: unknown) { return String(value || "").trim().slice(0, 120); }

function passkeyDescriptors(userId: string) {
  return q<{ credential_id: string; transports: string }>("SELECT credential_id,transports FROM passkeys WHERE user_id=?")
    .all(userId).map(row => ({ id: row.credential_id, transports: JSON.parse(row.transports || "[]") }));
}

export async function registrationCeremony(
  kind: Exclude<CeremonyKind, "login">, userId: string, name: string, inviteId: string | null = null,
) {
  const options = await generateRegistrationOptions({
    rpName: "RC", rpID: RP_ID, userName: name, userDisplayName: name,
    userID: new TextEncoder().encode(userId), attestationType: "none",
    excludeCredentials: kind === "add-passkey" ? passkeyDescriptors(userId) : [],
    authenticatorSelection: { residentKey: "required", requireResidentKey: true, userVerification: "required" },
  });
  const ceremonyId = id(), t = now();
  q(`INSERT INTO webauthn_challenges(id,challenge,kind,user_id,name,invite_id,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?)`).run(ceremonyId, options.challenge, kind, userId, name, inviteId, t, t + CEREMONY_TTL);
  return { ceremonyId, options };
}

export async function authenticationCeremony() {
  const options = await generateAuthenticationOptions({ rpID: RP_ID, userVerification: "required" });
  const ceremonyId = id(), t = now();
  q("INSERT INTO webauthn_challenges(id,challenge,kind,created_at,expires_at) VALUES(?,?,?,?,?)")
    .run(ceremonyId, options.challenge, "login", t, t + CEREMONY_TTL);
  return { ceremonyId, options };
}

export function takeCeremony(value: unknown, kind: CeremonyKind): CeremonyRow | null {
  const key = String(value || "");
  const row = q<CeremonyRow>("SELECT * FROM webauthn_challenges WHERE id=? AND kind=? AND expires_at>?").get(key, kind, now());
  if (key) q("DELETE FROM webauthn_challenges WHERE id=?").run(key);
  return row || null;
}

export async function verifyNewPasskey(ceremony: CeremonyRow, response: RegistrationResponseJSON) {
  const result = await verifyRegistrationResponse({
    response, expectedChallenge: ceremony.challenge, expectedOrigin: PUBLIC_URL,
    expectedRPID: RP_ID, requireUserVerification: true,
  });
  if (!result.verified || !result.registrationInfo) throw new Error("passkey verification failed");
  return result.registrationInfo.credential;
}

export function insertPasskey(userId: string, credential: WebAuthnCredential) {
  q(`INSERT INTO passkeys(id,user_id,credential_id,public_key,counter,transports,created_at) VALUES(?,?,?,?,?,?,?)`).run(
    id(), userId, credential.id, bytesToBase64(credential.publicKey), Number(credential.counter || 0),
    JSON.stringify(credential.transports || []), now(),
  );
}
