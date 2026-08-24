import {
  verifyAuthenticationResponse,
  type AuthenticationResponseJSON,
  type RegistrationResponseJSON,
} from "@simplewebauthn/server";
import { PUBLIC_URL, RP_ID, SESSION_TTL, SETUP_TOKEN, VERSION } from "./config";
import { User } from "./core";
import { db, id, now, opaque, q, sha } from "./db";
import { base64ToBytes } from "./encoding";
import { cookie, fail, json, sessionCookie } from "./http-utils";
import { HttpError } from "./errors";
import {
  authenticationCeremony, cleanName, insertPasskey, registrationCeremony, takeCeremony, verifyNewPasskey,
} from "./webauthn";

export type PasskeyView = { id: string; created_at: number; last_used: number | null };

export function setupAuthorized(req: Request) {
  if (!SETUP_TOKEN) return true;
  const token = cookie(req, "relay_setup");
  return !!token && sha(token) === sha(SETUP_TOKEN);
}

export function apiTokenUser(token: string): User | null {
  const row = q<any>(`SELECT u.id,u.name,a.id token_id FROM api_tokens a JOIN users u ON u.id=a.user_id
    WHERE a.token_hash=?`).get(sha(token));
  if (!row) return null;
  q("UPDATE api_tokens SET last_used=? WHERE id=?").run(now(), row.token_id);
  return { id: row.id, name: row.name };
}

export async function auth(req: Request): Promise<User | null> {
  const bearer = req.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1];
  if (bearer) return apiTokenUser(bearer);
  return cookieUser(req);
}

export async function cookieUser(req: Request): Promise<User | null> {
  const token = cookie(req, "relay_session");
  if (!token) return null;
  const row = q<any>(`SELECT u.id,u.name FROM auth_sessions s JOIN users u ON u.id=s.user_id
    WHERE s.token_hash=? AND s.expires_at>?`).get(sha(token), now());
  return row ? { id: row.id, name: row.name } : null;
}

export async function createLogin(userId: string) {
  const token = opaque("sess");
  q("INSERT INTO auth_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)")
    .run(sha(token), userId, now(), now() + SESSION_TTL);
  return token;
}

export function relayStatus(req: Request) {
  const count = q<{ count: number }>("SELECT count(*) count FROM users").get()?.count || 0;
  return { setupRequired: count === 0, setupAuthorized: count === 0 && setupAuthorized(req), version: VERSION };
}

export async function setupOptions(req: Request, value: unknown) {
  if ((q<{ count: number }>("SELECT count(*) count FROM users").get()?.count || 0) > 0) throw new HttpError(409, "setup already completed");
  if (!setupAuthorized(req)) throw new HttpError(403, "Open the Relay setup link first.");
  const name = cleanName(value);
  if (!name) throw new HttpError(400, "name required");
  return registrationCeremony("setup", id(), name);
}

export async function loginOptions() {
  if ((q<{ count: number }>("SELECT count(*) count FROM passkeys").get()?.count || 0) === 0) throw new HttpError(409, "no passkeys registered");
  return authenticationCeremony();
}

export async function tokenLogin(value: unknown) {
  const user = apiTokenUser(String(value || "").trim());
  if (!user) throw new HttpError(401, "invalid API token");
  return createLogin(user.id);
}

export async function registerOptions(inviteValue: unknown, nameValue: unknown) {
  const invite = String(inviteValue || "").trim(), name = cleanName(nameValue);
  const row = q<{ id: string }>("SELECT id FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?").get(sha(invite), now());
  if (!row) throw new HttpError(401, "invalid or expired invite");
  if (!name) throw new HttpError(400, "name required");
  return registrationCeremony("register", id(), name, row.id);
}

export function logout(req: Request) {
  const token = cookie(req, "relay_session");
  if (token) q("DELETE FROM auth_sessions WHERE token_hash=?").run(sha(token));
}

function requireHuman(req: Request, user: User) {
  return cookieUser(req).then(human => {
    if (!human || human.id !== user.id) throw new HttpError(401, "browser session required");
  });
}

export async function listPasskeys(req: Request, user: User): Promise<PasskeyView[]> {
  await requireHuman(req, user);
  return q<PasskeyView>("SELECT id,created_at,last_used FROM passkeys WHERE user_id=? ORDER BY created_at").all(user.id);
}

export async function addPasskeyOptions(req: Request, user: User) {
  await requireHuman(req, user);
  return registrationCeremony("add-passkey", user.id, user.name);
}

export async function verifyAddedPasskey(req: Request, user: User, ceremonyId: string, response: RegistrationResponseJSON) {
  await requireHuman(req, user);
  const ceremony = takeCeremony(ceremonyId, "add-passkey");
  if (!ceremony || ceremony.user_id !== user.id) throw new HttpError(410, "registration expired");
  try { insertPasskey(user.id, await verifyNewPasskey(ceremony, response)); }
  catch { throw new HttpError(401, "passkey verification failed"); }
}

export async function deletePasskey(req: Request, user: User, passkeyId: string) {
  await requireHuman(req, user);
  const count = q<{ count: number }>("SELECT count(*) count FROM passkeys WHERE user_id=?").get(user.id)?.count || 0;
  if (count <= 1) throw new HttpError(409, "add another passkey before removing your last one");
  if (!q("DELETE FROM passkeys WHERE id=? AND user_id=?").run(passkeyId, user.id).changes) throw new HttpError(404, "passkey not found");
}

export async function verifyLogin(ceremonyId: string, response: AuthenticationResponseJSON) {
  const ceremony = takeCeremony(ceremonyId, "login");
  if (!ceremony) return fail("authentication expired", 410);
  const row = q<any>(`SELECT p.*,u.name FROM passkeys p JOIN users u ON u.id=p.user_id WHERE p.credential_id=?`)
    .get(String(response.id || ""));
  if (!row) return fail("unknown passkey", 401);
  try {
    const result = await verifyAuthenticationResponse({
      response, expectedChallenge: ceremony.challenge, expectedOrigin: PUBLIC_URL,
      expectedRPID: RP_ID, requireUserVerification: true,
      credential: { id: row.credential_id, publicKey: base64ToBytes(row.public_key),
        counter: Number(row.counter || 0), transports: JSON.parse(row.transports || "[]") },
    });
    if (!result.verified) return fail("passkey verification failed", 401);
    q("UPDATE passkeys SET counter=?,last_used=? WHERE id=?").run(result.authenticationInfo.newCounter, now(), row.id);
    const token = await createLogin(row.user_id);
    return json({ ok: true }, 200, { "set-cookie": sessionCookie(token) });
  } catch { return fail("passkey verification failed", 401); }
}

export async function verifyNewUser(kind: "setup" | "register", ceremonyId: string, response: RegistrationResponseJSON) {
  const ceremony = takeCeremony(ceremonyId, kind);
  if (!ceremony) return fail("registration expired", 410);
  if (!ceremony.user_id || !ceremony.name) return fail("registration expired", 410);
  if (kind === "setup" && (q<any>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return fail("setup already completed", 409);
  const invite = kind === "register"
    ? q<any>("SELECT * FROM workspace_invites WHERE id=? AND used_at IS NULL AND expires_at>?").get(ceremony.invite_id, now()) : null;
  if (kind === "register" && !invite) return fail("invalid or expired invite", 401);
  let credential;
  try { credential = await verifyNewPasskey(ceremony, response); }
  catch { return fail("passkey verification failed", 401); }
  const userId = ceremony.user_id, t = now(), workspaceId = kind === "setup" ? id() : String(invite.workspace_id);
  db.transaction(() => {
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, ceremony.name, t);
    insertPasskey(userId, credential);
    if (kind === "setup") {
      q("INSERT INTO workspaces VALUES(?,?,?,?)").run(workspaceId, "Personal", userId, t);
      q("INSERT INTO workspace_members VALUES(?,?,?,?)").run(workspaceId, userId, "owner", t);
    } else {
      q("INSERT INTO workspace_members VALUES(?,?,?,?)").run(workspaceId, userId, invite.role, t);
      q("UPDATE workspace_invites SET used_at=? WHERE id=?").run(t, invite.id);
    }
  })();
  const token = await createLogin(userId);
  return json({ ok: true }, 201, { "set-cookie": sessionCookie(token) });
}
