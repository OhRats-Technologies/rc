import {
  generateAuthenticationOptions,
  generateRegistrationOptions,
  verifyAuthenticationResponse,
  verifyRegistrationResponse,
} from "@simplewebauthn/server";
import { CEREMONY_TTL, PUBLIC_URL, RP_ID, SESSION_TTL, SETUP_TOKEN, VERSION } from "./config";
import { User, userWorkspaces } from "./core";
import { db, id, now, opaque, q, sha } from "./db";
import { body, cookie, fail, json, sessionCookie } from "./http-utils";

export function setupAuthorized(req: Request) {
  if (!SETUP_TOKEN) return true;
  const token = cookie(req, "relay_setup");
  return !!token && sha(token) === sha(SETUP_TOKEN);
}

export async function auth(req: Request): Promise<User | null> {
  const bearer = req.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1];
  if (bearer) {
    const row = q<any>(`SELECT u.id,u.name,a.id token_id FROM api_tokens a JOIN users u ON u.id=a.user_id
      WHERE a.token_hash=?`).get(sha(bearer));
    if (row) {
      q("UPDATE api_tokens SET last_used=? WHERE id=?").run(now(), row.token_id);
      return { id: row.id, name: row.name };
    }
  }
  return cookieUser(req);
}

export async function cookieUser(req: Request): Promise<User | null> {
  const token = cookie(req, "relay_session");
  if (!token) return null;
  const row = q<any>(`SELECT u.id,u.name FROM auth_sessions s JOIN users u ON u.id=s.user_id
    WHERE s.token_hash=? AND s.expires_at>?`).get(sha(token), now());
  return row ? { id: row.id, name: row.name } : null;
}

async function createLogin(userId: string) {
  const token = opaque("sess");
  q("INSERT INTO auth_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)")
    .run(sha(token), userId, now(), now() + SESSION_TTL);
  return token;
}

function cleanName(value: unknown) { return String(value || "").trim().slice(0, 120); }
function passkeyDescriptors(userId: string) {
  return q<any>("SELECT credential_id,transports FROM passkeys WHERE user_id=?").all(userId).map((row: any) => ({
    id: row.credential_id, transports: JSON.parse(row.transports || "[]"),
  }));
}

async function registrationCeremony(kind: "setup" | "register" | "add-passkey", userId: string, name: string, inviteId: string | null = null) {
  const options = await generateRegistrationOptions({
    rpName: "Relay", rpID: RP_ID, userName: name, userDisplayName: name,
    userID: new TextEncoder().encode(userId), attestationType: "none",
    excludeCredentials: kind === "add-passkey" ? passkeyDescriptors(userId) : [],
    authenticatorSelection: { residentKey: "required", requireResidentKey: true, userVerification: "required" },
  });
  const ceremonyId = id(), t = now();
  q(`INSERT INTO webauthn_challenges(id,challenge,kind,user_id,name,invite_id,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?)`).run(ceremonyId, options.challenge, kind, userId, name, inviteId, t, t + CEREMONY_TTL);
  return { ceremonyId, options };
}

async function authenticationCeremony() {
  const options = await generateAuthenticationOptions({ rpID: RP_ID, userVerification: "required" });
  const ceremonyId = id(), t = now();
  q("INSERT INTO webauthn_challenges(id,challenge,kind,created_at,expires_at) VALUES(?,?,?,?,?)")
    .run(ceremonyId, options.challenge, "login", t, t + CEREMONY_TTL);
  return { ceremonyId, options };
}

function takeCeremony(value: unknown, kind: string) {
  const key = String(value || "");
  const row = q<any>("SELECT * FROM webauthn_challenges WHERE id=? AND kind=? AND expires_at>?").get(key, kind, now());
  if (key) q("DELETE FROM webauthn_challenges WHERE id=?").run(key);
  return row || null;
}

async function verifyNewPasskey(ceremony: any, response: any) {
  const result = await verifyRegistrationResponse({
    response, expectedChallenge: ceremony.challenge, expectedOrigin: PUBLIC_URL,
    expectedRPID: RP_ID, requireUserVerification: true,
  });
  if (!result.verified || !result.registrationInfo) throw new Error("passkey verification failed");
  return result.registrationInfo.credential;
}

function insertPasskey(userId: string, credential: any) {
  q(`INSERT INTO passkeys(id,user_id,credential_id,public_key,counter,transports,created_at) VALUES(?,?,?,?,?,?,?)`).run(
    id(), userId, credential.id, Buffer.from(credential.publicKey).toString("base64"), Number(credential.counter || 0),
    JSON.stringify(credential.transports || []), now()
  );
}

async function loginVerify(req: Request) {
  const input = await body(req), ceremony = takeCeremony(input.ceremonyId, "login");
  if (!ceremony) return fail("authentication expired", 410);
  const row = q<any>(`SELECT p.*,u.name FROM passkeys p JOIN users u ON u.id=p.user_id WHERE p.credential_id=?`)
    .get(String(input.response?.id || ""));
  if (!row) return fail("unknown passkey", 401);
  try {
    const result = await verifyAuthenticationResponse({
      response: input.response, expectedChallenge: ceremony.challenge, expectedOrigin: PUBLIC_URL,
      expectedRPID: RP_ID, requireUserVerification: true,
      credential: { id: row.credential_id, publicKey: Buffer.from(row.public_key, "base64"),
        counter: Number(row.counter || 0), transports: JSON.parse(row.transports || "[]") },
    });
    if (!result.verified) return fail("passkey verification failed", 401);
    q("UPDATE passkeys SET counter=?,last_used=? WHERE id=?").run(result.authenticationInfo.newCounter, now(), row.id);
    const token = await createLogin(row.user_id);
    return json({ ok: true }, 200, { "set-cookie": sessionCookie(token) });
  } catch { return fail("passkey verification failed", 401); }
}

async function newUserVerify(req: Request, kind: "setup" | "register") {
  const input = await body(req), ceremony = takeCeremony(input.ceremonyId, kind);
  if (!ceremony) return fail("registration expired", 410);
  if (kind === "setup" && (q<any>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return fail("setup already completed", 409);
  const invite = kind === "register"
    ? q<any>("SELECT * FROM workspace_invites WHERE id=? AND used_at IS NULL AND expires_at>?").get(ceremony.invite_id, now()) : null;
  if (kind === "register" && !invite) return fail("invalid or expired invite", 401);
  let credential;
  try { credential = await verifyNewPasskey(ceremony, input.response); }
  catch { return fail("passkey verification failed", 401); }
  const userId = ceremony.user_id, t = now(), workspaceId = kind === "setup" ? id() : invite.workspace_id;
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

export async function handlePublicAuth(req: Request, path: string): Promise<Response | null> {
  if (path === "/api/v1/status" && req.method === "GET") {
    const count = q<any>("SELECT count(*) count FROM users").get()?.count || 0;
    return json({ setupRequired: count === 0, setupAuthorized: count === 0 && setupAuthorized(req), version: VERSION });
  }
  if (["/api/v1/auth/setup", "/api/v1/auth/login", "/api/v1/auth/register"].includes(path)) {
    return fail("Relay was updated. Refresh this page and try again.", 409);
  }
  if (path === "/api/v1/auth/setup/options" && req.method === "POST") {
    if ((q<any>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return fail("setup already completed", 409);
    if (!setupAuthorized(req)) return fail("Open the Relay setup link first.", 403);
    const input = await body(req), name = cleanName(input.name);
    return name ? json(await registrationCeremony("setup", id(), name), 201) : fail("name required");
  }
  if (path === "/api/v1/auth/setup/verify" && req.method === "POST") return newUserVerify(req, "setup");
  if (path === "/api/v1/auth/login/options" && req.method === "POST") {
    if ((q<any>("SELECT count(*) count FROM passkeys").get()?.count || 0) === 0) return fail("no passkeys registered", 409);
    return json(await authenticationCeremony(), 201);
  }
  if (path === "/api/v1/auth/login/verify" && req.method === "POST") return loginVerify(req);
  if (path === "/api/v1/auth/register/options" && req.method === "POST") {
    const input = await body(req), invite = String(input.invite || "").trim(), name = cleanName(input.name);
    const row = q<any>("SELECT * FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?").get(sha(invite), now());
    if (!row) return fail("invalid or expired invite", 401);
    return name ? json(await registrationCeremony("register", id(), name, row.id), 201) : fail("name required");
  }
  if (path === "/api/v1/auth/register/verify" && req.method === "POST") return newUserVerify(req, "register");
  if (path === "/api/v1/auth/logout" && req.method === "POST") {
    const token = cookie(req, "relay_session");
    if (token) q("DELETE FROM auth_sessions WHERE token_hash=?").run(sha(token));
    return json({ ok: true }, 200, { "set-cookie": sessionCookie("", 0) });
  }
  return null;
}

export async function handleAccount(req: Request, path: string, user: User): Promise<Response | null> {
  const human = await cookieUser(req);
  if (path === "/api/v1/me" && req.method === "GET") return json({ user, workspaces: userWorkspaces(user.id) });
  if (path === "/api/v1/passkeys" && req.method === "GET") {
    if (!human || human.id !== user.id) return fail("browser session required", 401);
    return json({ passkeys: q<any>("SELECT id,created_at,last_used FROM passkeys WHERE user_id=? ORDER BY created_at").all(user.id) });
  }
  if (path === "/api/v1/passkeys/options" && req.method === "POST") {
    if (!human || human.id !== user.id) return fail("browser session required", 401);
    return json(await registrationCeremony("add-passkey", user.id, user.name), 201);
  }
  if (path === "/api/v1/passkeys/verify" && req.method === "POST") {
    if (!human || human.id !== user.id) return fail("browser session required", 401);
    const input = await body(req), ceremony = takeCeremony(input.ceremonyId, "add-passkey");
    if (!ceremony || ceremony.user_id !== user.id) return fail("registration expired", 410);
    try { insertPasskey(user.id, await verifyNewPasskey(ceremony, input.response)); return json({ ok: true }, 201); }
    catch { return fail("passkey verification failed", 401); }
  }
  const match = path.match(/^\/api\/v1\/passkeys\/([^/]+)$/);
  if (match && req.method === "DELETE") {
    if (!human || human.id !== user.id) return fail("browser session required", 401);
    const removed = q("DELETE FROM passkeys WHERE id=? AND user_id=?").run(match[1], user.id);
    return removed.changes ? json({ ok: true }) : fail("passkey not found", 404);
  }
  return null;
}
