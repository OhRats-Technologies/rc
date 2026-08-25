import { generateAuthenticationOptions, verifyAuthenticationResponse, type AuthenticationResponseJSON } from "@simplewebauthn/server";
import { PUBLIC_URL, RP_ID } from "./config";
import type { User } from "./core";
import { id, now, opaque, q, sha } from "./db";
import { base64ToBytes } from "./encoding";
import { HttpError } from "./errors";
import { cookie } from "./http-utils";

const AUTH_TTL = 5 * 60_000;
const TOKEN_TTL = 2 * 60_000;
const RECENT_SESSION_TTL = 5 * 60_000;

function descriptors(userId: string) {
  return q<{ credential_id: string; transports: string }>("SELECT credential_id,transports FROM passkeys WHERE user_id=?")
    .all(userId).map(row => ({ id: row.credential_id, transports: JSON.parse(row.transports || "[]") }));
}

export async function stepUpOptions(user: User) {
  const options = await generateAuthenticationOptions({
    rpID: RP_ID, userVerification: "required", allowCredentials: descriptors(user.id),
  });
  const authorizationId = id(), t = now();
  q("INSERT INTO step_up_authorizations(id,user_id,challenge,created_at,expires_at) VALUES(?,?,?,?,?)")
    .run(authorizationId, user.id, options.challenge, t, t + AUTH_TTL);
  return { authorizationId, options };
}

export async function verifyStepUp(user: User, authorizationId: string, response: AuthenticationResponseJSON) {
  const row = q<{ id: string; challenge: string }>(
    "SELECT id,challenge FROM step_up_authorizations WHERE id=? AND user_id=? AND expires_at>?"
  ).get(authorizationId, user.id, now());
  if (!row) throw new HttpError(410, "passkey step-up expired");
  q("DELETE FROM step_up_authorizations WHERE id=?").run(row.id);
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
  const token = opaque("step"), t = now();
  q("INSERT INTO step_up_tokens(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)").run(sha(token), user.id, t, t + TOKEN_TTL);
  return { token, expiresAt: t + TOKEN_TTL };
}

export function consumeStepUp(request: Request, user: User) {
  const token = request.headers.get("x-rc-step-up") || "";
  if (!token || q("DELETE FROM step_up_tokens WHERE token_hash=? AND user_id=? AND expires_at>?")
    .run(sha(token), user.id, now()).changes !== 1) throw new HttpError(401, "fresh passkey verification required");
}

export function recentPasskeySession(request: Request, user: User) {
  const token = cookie(request, "rc_session");
  if (!token) return false;
  const t = now();
  return Boolean(q("SELECT 1 ok FROM auth_sessions WHERE token_hash=? AND user_id=? AND created_at>? AND expires_at>?")
    .get(sha(token), user.id, t - RECENT_SESSION_TTL, t));
}

export function consumeStepUpOrRecentSession(request: Request, user: User) {
  if (recentPasskeySession(request, user)) return;
  consumeStepUp(request, user);
}
