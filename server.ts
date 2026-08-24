import { Database } from "bun:sqlite";
import { createHash, createPublicKey, randomBytes, verify as verifySignature } from "node:crypto";
import { existsSync, mkdirSync } from "node:fs";
import { extname, join } from "node:path";
import {
  generateAuthenticationOptions,
  generateRegistrationOptions,
  verifyAuthenticationResponse,
  verifyRegistrationResponse,
} from "@simplewebauthn/server";

const PORT = Number(process.env.PORT || 3000);
const DATA_DIR = process.env.DATA_DIR || "./data";
const PUBLIC_URL = (process.env.PUBLIC_URL || `http://localhost:${PORT}`).replace(/\/$/, "");
const SETUP_TOKEN = String(process.env.RELAY_SETUP_TOKEN || "").trim();
const RP_ID = new URL(PUBLIC_URL).hostname;
const RP_NAME = "Relay";
const DB_PATH = join(DATA_DIR, "relay.db");
const SESSION_TTL = 30 * 24 * 60 * 60 * 1000;
const TOKEN_TTL = 24 * 60 * 60 * 1000;
const SETUP_COOKIE_TTL = 15 * 60;
const CEREMONY_TTL = 5 * 60 * 1000;

mkdirSync(DATA_DIR, { recursive: true });
const db = new Database(DB_PATH, { create: true, strict: true });
db.exec("PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;");

db.exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS passkeys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id TEXT NOT NULL UNIQUE,
  public_key TEXT NOT NULL,
  counter INTEGER NOT NULL DEFAULT 0,
  transports TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  last_used INTEGER
);
CREATE TABLE IF NOT EXISTS webauthn_challenges (
  id TEXT PRIMARY KEY,
  challenge TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK(kind IN ('setup','register','login','add-passkey')),
  user_id TEXT,
  name TEXT,
  invite_id TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_members (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK(role IN ('owner','member','viewer')),
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, user_id)
);
CREATE TABLE IF NOT EXISTS fleets (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  hostname TEXT NOT NULL,
  platform TEXT NOT NULL,
  arch TEXT NOT NULL,
  public_key TEXT NOT NULL UNIQUE,
  agent_version TEXT NOT NULL,
  capabilities TEXT NOT NULL DEFAULT '[]',
  last_seen INTEGER,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS fleet_devices (
  fleet_id TEXT NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  permissions TEXT NOT NULL DEFAULT '["shell"]',
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (fleet_id, device_id)
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id),
  type TEXT NOT NULL DEFAULT 'shell',
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','closed')),
  created_at INTEGER NOT NULL,
  closed_at INTEGER
);
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'shell',
  payload TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sent','completed','failed','cancelled')),
  result TEXT,
  exit_code INTEGER,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,
  started_at INTEGER,
  completed_at INTEGER
);
CREATE TABLE IF NOT EXISTS auth_sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  last_used INTEGER
);
CREATE TABLE IF NOT EXISTS workspace_invites (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL CHECK(role IN ('member','viewer')),
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at INTEGER
);
CREATE TABLE IF NOT EXISTS enrollment_tokens (
  id TEXT PRIMARY KEY,
  fleet_id TEXT NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at INTEGER
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_members_user ON workspace_members(user_id);
CREATE INDEX IF NOT EXISTS idx_passkeys_user ON passkeys(user_id);
CREATE INDEX IF NOT EXISTS idx_fleets_workspace ON fleets(workspace_id);
CREATE INDEX IF NOT EXISTS idx_fleet_devices_device ON fleet_devices(device_id);
CREATE INDEX IF NOT EXISTS idx_sessions_device ON sessions(device_id);
CREATE INDEX IF NOT EXISTS idx_jobs_device_status ON jobs(device_id, status);
CREATE INDEX IF NOT EXISTS idx_events_workspace ON events(workspace_id, created_at DESC);
`);

type User = { id: string; name: string };
type AgentData = { kind: "agent"; deviceId: string };
type Role = "owner" | "member" | "viewer";

const agents = new Map<string, ServerWebSocket<AgentData>>();

const q = <T = any>(sql: string) => db.query<T, any[]>(sql);
const now = () => Date.now();
const id = () => crypto.randomUUID();
const sha = (value: string) => createHash("sha256").update(value).digest("hex");
const opaque = (prefix: string) => `${prefix}_${randomBytes(32).toString("base64url")}`;

if (!q<any>(`SELECT version FROM schema_migrations WHERE version=1`).get()) {
  q(`INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)`).run(now());
}

const userColumns = q<any>(`PRAGMA table_info(users)`).all();
if (userColumns.some((column: any) => column.name === "email" || column.name === "password_hash")) {
  db.exec("PRAGMA foreign_keys = OFF");
  try {
    db.transaction(() => {
      db.exec(`
        CREATE TABLE users_v2 (
          id TEXT PRIMARY KEY,
          name TEXT NOT NULL,
          created_at INTEGER NOT NULL
        );
        INSERT INTO users_v2(id,name,created_at) SELECT id,name,created_at FROM users;
        DROP TABLE users;
        ALTER TABLE users_v2 RENAME TO users;
      `);
    })();
  } finally {
    db.exec("PRAGMA foreign_keys = ON");
  }
}
if (!q<any>(`SELECT version FROM schema_migrations WHERE version=2`).get()) {
  q(`INSERT INTO schema_migrations(version,applied_at) VALUES(2,?)`).run(now());
}

function json(data: unknown, status = 200, headers: HeadersInit = {}) {
  return Response.json(data, { status, headers: { "cache-control": "no-store", ...headers } });
}

function fail(message: string, status = 400) {
  return json({ error: message }, status);
}

async function body(req: Request) {
  const length = Number(req.headers.get("content-length") || 0);
  if (length > 1024 * 1024) throw new Error("request too large");
  return await req.json();
}

function cookie(req: Request, name: string) {
  const raw = req.headers.get("cookie") || "";
  for (const part of raw.split(";")) {
    const [key, ...rest] = part.trim().split("=");
    if (key === name) return decodeURIComponent(rest.join("="));
  }
  return "";
}

function sessionCookie(token: string, maxAge = Math.floor(SESSION_TTL / 1000)) {
  return `relay_session=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}${PUBLIC_URL.startsWith("https://") ? "; Secure" : ""}`;
}

function setupCookie(token: string) {
  return `relay_setup=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Strict; Max-Age=${SETUP_COOKIE_TTL}${PUBLIC_URL.startsWith("https://") ? "; Secure" : ""}`;
}

function setupAuthorized(req: Request) {
  if (!SETUP_TOKEN) return true;
  const token = cookie(req, "relay_setup");
  return !!token && sha(token) === sha(SETUP_TOKEN);
}

async function auth(req: Request): Promise<User | null> {
  const bearer = req.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1];
  if (bearer) {
    const row = q<any>(`SELECT u.id,u.name,a.id token_id FROM api_tokens a JOIN users u ON u.id=a.user_id WHERE a.token_hash=?`).get(sha(bearer));
    if (row) {
      q(`UPDATE api_tokens SET last_used=? WHERE id=?`).run(now(), row.token_id);
      return { id: row.id, name: row.name };
    }
  }
  const token = cookie(req, "relay_session");
  if (!token) return null;
  const row = q<any>(`SELECT u.id,u.name FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`).get(sha(token), now());
  return row ? { id: row.id, name: row.name } : null;
}

async function cookieUser(req: Request): Promise<User | null> {
  const token = cookie(req, "relay_session");
  if (!token) return null;
  const row = q<any>(`SELECT u.id,u.name FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`).get(sha(token), now());
  return row ? { id: row.id, name: row.name } : null;
}

function roleFor(userId: string, workspaceId: string): Role | null {
  const row = q<any>(`SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?`).get(workspaceId, userId);
  return row?.role || null;
}

function canWrite(role: Role | null) { return role === "owner" || role === "member"; }

function deviceRole(userId: string, deviceId: string): Role | null {
  const row = q<any>(`
    SELECT wm.role FROM workspace_members wm
    JOIN fleets f ON f.workspace_id=wm.workspace_id
    JOIN fleet_devices fd ON fd.fleet_id=f.id
    WHERE wm.user_id=? AND fd.device_id=? LIMIT 1
  `).get(userId, deviceId);
  return row?.role || null;
}

function devicePermission(userId: string, deviceId: string, capability: string) {
  const rows = q<any>(`
    SELECT fd.permissions FROM workspace_members wm
    JOIN fleets f ON f.workspace_id=wm.workspace_id
    JOIN fleet_devices fd ON fd.fleet_id=f.id
    WHERE wm.user_id=? AND fd.device_id=?
  `).all(userId, deviceId);
  return rows.some((row: any) => {
    try { return JSON.parse(row.permissions || "[]").includes(capability); }
    catch { return false; }
  });
}

function sessionAccess(userId: string, sessionId: string) {
  const row = q<any>(`SELECT device_id FROM sessions WHERE id=?`).get(sessionId);
  return row ? { deviceId: row.device_id, role: deviceRole(userId, row.device_id) } : null;
}

function logEvent(kind: string, workspaceId: string | null, userId: string | null, deviceId: string | null, detail: unknown = {}) {
  q(`INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES(?,?,?,?,?,?)`).run(workspaceId, userId, deviceId, kind, JSON.stringify(detail), now());
}

function userWorkspaces(userId: string) {
  return q<any>(`
    SELECT w.id,w.name,wm.role,w.created_at FROM workspaces w
    JOIN workspace_members wm ON wm.workspace_id=w.id
    WHERE wm.user_id=? ORDER BY w.created_at
  `).all(userId);
}

function dashboard(user: User, requestedWorkspace?: string | null) {
  const workspaces = userWorkspaces(user.id);
  const workspaceId = requestedWorkspace && workspaces.some((w: any) => w.id === requestedWorkspace) ? requestedWorkspace : workspaces[0]?.id;
  if (!workspaceId) return { user, workspaces, workspace: null, fleets: [], devices: [], events: [] };
  const workspace = workspaces.find((w: any) => w.id === workspaceId);
  const fleets = q<any>(`SELECT id,name,created_at FROM fleets WHERE workspace_id=? ORDER BY created_at`).all(workspaceId);
  const devices = q<any>(`
    SELECT d.id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,d.last_seen,d.created_at,
           group_concat(f.name, ', ') fleets
    FROM devices d
    JOIN fleet_devices fd ON fd.device_id=d.id
    JOIN fleets f ON f.id=fd.fleet_id
    WHERE f.workspace_id=?
    GROUP BY d.id ORDER BY d.name
  `).all(workspaceId).map((d: any) => ({ ...d, online: agents.has(d.id), capabilities: JSON.parse(d.capabilities || "[]") }));
  const events = q<any>(`SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 30`).all(workspaceId).map((e: any) => ({ ...e, detail: JSON.parse(e.detail || "{}") }));
  return { user, workspaces, workspace, fleets, devices, events };
}

function checkOrigin(req: Request) {
  if (["GET", "HEAD", "OPTIONS"].includes(req.method)) return true;
  if (req.headers.has("authorization")) return true;
  const origin = req.headers.get("origin");
  if (!origin) return true;
  return origin === new URL(req.url).origin || origin === PUBLIC_URL;
}

async function createLogin(userId: string) {
  const token = opaque("sess");
  q(`INSERT INTO auth_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)`).run(sha(token), userId, now(), now() + SESSION_TTL);
  return token;
}

function cleanName(value: unknown) {
  return String(value || "").trim().slice(0, 120);
}

function passkeyDescriptors(userId: string) {
  return q<any>(`SELECT credential_id,transports FROM passkeys WHERE user_id=?`).all(userId).map((row: any) => ({
    id: row.credential_id,
    transports: JSON.parse(row.transports || "[]"),
  }));
}

async function registrationCeremony(kind: "setup" | "register" | "add-passkey", userId: string, name: string, inviteId: string | null = null) {
  const options = await generateRegistrationOptions({
    rpName: RP_NAME,
    rpID: RP_ID,
    userName: name,
    userDisplayName: name,
    userID: new TextEncoder().encode(userId),
    attestationType: "none",
    excludeCredentials: kind === "add-passkey" ? passkeyDescriptors(userId) : [],
    authenticatorSelection: {
      residentKey: "required",
      requireResidentKey: true,
      userVerification: "required",
    },
  });
  const ceremonyId = id(), t = now();
  q(`INSERT INTO webauthn_challenges(id,challenge,kind,user_id,name,invite_id,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)`).run(
    ceremonyId, options.challenge, kind, userId, name, inviteId, t, t + CEREMONY_TTL
  );
  return { ceremonyId, options };
}

async function authenticationCeremony() {
  const options = await generateAuthenticationOptions({
    rpID: RP_ID,
    userVerification: "required",
  });
  const ceremonyId = id(), t = now();
  q(`INSERT INTO webauthn_challenges(id,challenge,kind,created_at,expires_at) VALUES(?,?,?,?,?)`).run(
    ceremonyId, options.challenge, "login", t, t + CEREMONY_TTL
  );
  return { ceremonyId, options };
}

function takeCeremony(ceremonyId: unknown, kind: string) {
  const key = String(ceremonyId || "");
  const row = q<any>(`SELECT * FROM webauthn_challenges WHERE id=? AND kind=? AND expires_at>?`).get(key, kind, now());
  if (key) q(`DELETE FROM webauthn_challenges WHERE id=?`).run(key);
  return row || null;
}

async function verifyNewPasskey(ceremony: any, response: any) {
  const verification = await verifyRegistrationResponse({
    response,
    expectedChallenge: ceremony.challenge,
    expectedOrigin: PUBLIC_URL,
    expectedRPID: RP_ID,
    requireUserVerification: true,
  });
  if (!verification.verified || !verification.registrationInfo) throw new Error("passkey verification failed");
  return verification.registrationInfo.credential;
}

function insertPasskey(userId: string, credential: any) {
  q(`INSERT INTO passkeys(id,user_id,credential_id,public_key,counter,transports,created_at) VALUES(?,?,?,?,?,?,?)`).run(
    id(), userId, credential.id, Buffer.from(credential.publicKey).toString("base64"), Number(credential.counter || 0),
    JSON.stringify(credential.transports || []), now()
  );
}

async function handleAPI(req: Request, url: URL): Promise<Response> {
  if (!checkOrigin(req)) return fail("invalid origin", 403);
  const path = url.pathname;

  if (path === "/api/v1/health" && req.method === "GET") {
    return json({ ok: true, version: "0.1.0", agents: agents.size });
  }
  if (path === "/api/v1/status" && req.method === "GET") {
    const count = q<any>(`SELECT count(*) count FROM users`).get()?.count || 0;
    return json({ setupRequired: count === 0, setupAuthorized: count === 0 && setupAuthorized(req), version: "0.1.0" });
  }
  if (["/api/v1/auth/setup", "/api/v1/auth/login", "/api/v1/auth/register"].includes(path)) {
    return fail("Relay was updated. Refresh this page and try again.", 409);
  }
  if (path === "/api/v1/auth/setup/options" && req.method === "POST") {
    if ((q<any>(`SELECT count(*) count FROM users`).get()?.count || 0) > 0) return fail("setup already completed", 409);
    if (!setupAuthorized(req)) return fail("Open the Relay setup link first.", 403);
    const input = await body(req);
    const name = cleanName(input.name);
    if (!name) return fail("name required");
    return json(await registrationCeremony("setup", id(), name), 201);
  }
  if (path === "/api/v1/auth/setup/verify" && req.method === "POST") {
    const input = await body(req);
    const ceremony = takeCeremony(input.ceremonyId, "setup");
    if (!ceremony) return fail("registration expired", 410);
    if ((q<any>(`SELECT count(*) count FROM users`).get()?.count || 0) > 0) return fail("setup already completed", 409);
    let credential;
    try { credential = await verifyNewPasskey(ceremony, input.response); }
    catch { return fail("passkey verification failed", 401); }
    const userId = ceremony.user_id, workspaceId = id(), fleetId = id(), t = now();
    db.transaction(() => {
      if ((q<any>(`SELECT count(*) count FROM users`).get()?.count || 0) > 0) throw new Error("setup already completed");
      q(`INSERT INTO users(id,name,created_at) VALUES(?,?,?)`).run(userId, ceremony.name, t);
      insertPasskey(userId, credential);
      q(`INSERT INTO workspaces VALUES(?,?,?,?)`).run(workspaceId, "Personal", userId, t);
      q(`INSERT INTO workspace_members VALUES(?,?,?,?)`).run(workspaceId, userId, "owner", t);
      q(`INSERT INTO fleets VALUES(?,?,?,?)`).run(fleetId, workspaceId, "Default", t);
    })();
    logEvent("workspace.created", workspaceId, userId, null, { name: "Personal" });
    const token = await createLogin(userId);
    return json({ ok: true }, 201, { "set-cookie": sessionCookie(token) });
  }
  if (path === "/api/v1/auth/login/options" && req.method === "POST") {
    if ((q<any>(`SELECT count(*) count FROM passkeys`).get()?.count || 0) === 0) return fail("no passkeys registered", 409);
    return json(await authenticationCeremony(), 201);
  }
  if (path === "/api/v1/auth/login/verify" && req.method === "POST") {
    const input = await body(req);
    const ceremony = takeCeremony(input.ceremonyId, "login");
    if (!ceremony) return fail("authentication expired", 410);
    const credentialId = String(input.response?.id || "");
    const row = q<any>(`SELECT p.*,u.name FROM passkeys p JOIN users u ON u.id=p.user_id WHERE p.credential_id=?`).get(credentialId);
    if (!row) return fail("unknown passkey", 401);
    let verification;
    try {
      verification = await verifyAuthenticationResponse({
        response: input.response,
        expectedChallenge: ceremony.challenge,
        expectedOrigin: PUBLIC_URL,
        expectedRPID: RP_ID,
        requireUserVerification: true,
        credential: {
          id: row.credential_id,
          publicKey: Buffer.from(row.public_key, "base64"),
          counter: Number(row.counter || 0),
          transports: JSON.parse(row.transports || "[]"),
        },
      });
    } catch { return fail("passkey verification failed", 401); }
    if (!verification.verified) return fail("passkey verification failed", 401);
    q(`UPDATE passkeys SET counter=?,last_used=? WHERE id=?`).run(verification.authenticationInfo.newCounter, now(), row.id);
    const token = await createLogin(row.user_id);
    return json({ ok: true }, 200, { "set-cookie": sessionCookie(token) });
  }
  if (path === "/api/v1/auth/register/options" && req.method === "POST") {
    const input = await body(req);
    const invite = String(input.invite || "").trim();
    const inviteRow = q<any>(`SELECT * FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?`).get(sha(invite), now());
    if (!inviteRow) return fail("invalid or expired invite", 401);
    const name = cleanName(input.name);
    if (!name) return fail("name required");
    return json(await registrationCeremony("register", id(), name, inviteRow.id), 201);
  }
  if (path === "/api/v1/auth/register/verify" && req.method === "POST") {
    const input = await body(req);
    const ceremony = takeCeremony(input.ceremonyId, "register");
    if (!ceremony) return fail("registration expired", 410);
    const inviteRow = q<any>(`SELECT * FROM workspace_invites WHERE id=? AND used_at IS NULL AND expires_at>?`).get(ceremony.invite_id, now());
    if (!inviteRow) return fail("invalid or expired invite", 401);
    let credential;
    try { credential = await verifyNewPasskey(ceremony, input.response); }
    catch { return fail("passkey verification failed", 401); }
    const userId = ceremony.user_id, t = now();
    db.transaction(() => {
      q(`INSERT INTO users(id,name,created_at) VALUES(?,?,?)`).run(userId, ceremony.name, t);
      insertPasskey(userId, credential);
      q(`INSERT INTO workspace_members VALUES(?,?,?,?)`).run(inviteRow.workspace_id, userId, inviteRow.role, t);
      q(`UPDATE workspace_invites SET used_at=? WHERE id=?`).run(t, inviteRow.id);
    })();
    logEvent("member.joined", inviteRow.workspace_id, userId, null, { role: inviteRow.role });
    const token = await createLogin(userId);
    return json({ ok: true }, 201, { "set-cookie": sessionCookie(token) });
  }
  if (path === "/api/v1/auth/logout" && req.method === "POST") {
    const token = cookie(req, "relay_session");
    if (token) q(`DELETE FROM auth_sessions WHERE token_hash=?`).run(sha(token));
    return json({ ok: true }, 200, { "set-cookie": sessionCookie("", 0) });
  }

  if (path === "/api/v1/agent/enroll" && req.method === "POST") {
    const input = await body(req);
    const token = String(input.token || "").trim();
    const enrollment = q<any>(`
      SELECT et.*,f.workspace_id FROM enrollment_tokens et JOIN fleets f ON f.id=et.fleet_id
      WHERE et.token_hash=? AND et.used_at IS NULL AND et.expires_at>?
    `).get(sha(token), now());
    if (!enrollment) return fail("invalid or expired enrollment token", 401);
    const publicKey = String(input.publicKey || "");
    if (!publicKey.includes("BEGIN PUBLIC KEY")) return fail("invalid public key");
    if (q(`SELECT id FROM devices WHERE public_key=?`).get(publicKey)) return fail("device key already enrolled", 409);
    const deviceId = id(), t = now();
    db.transaction(() => {
      q(`INSERT INTO devices(id,name,hostname,platform,arch,public_key,agent_version,capabilities,last_seen,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`).run(
        deviceId, String(input.name || input.hostname || "Device").slice(0, 120), String(input.hostname || "unknown").slice(0, 255),
        String(input.platform || "unknown").slice(0, 40), String(input.arch || "unknown").slice(0, 40), publicKey,
        String(input.agentVersion || "unknown").slice(0, 40), JSON.stringify(Array.isArray(input.capabilities) ? input.capabilities : []), t, t
      );
      q(`INSERT INTO fleet_devices(fleet_id,device_id,permissions,joined_at) VALUES(?,?,?,?)`).run(enrollment.fleet_id, deviceId, JSON.stringify(["shell"]), t);
      q(`UPDATE enrollment_tokens SET used_at=? WHERE id=?`).run(t, enrollment.id);
    })();
    logEvent("device.enrolled", enrollment.workspace_id, enrollment.created_by, deviceId, { fleetId: enrollment.fleet_id });
    return json({ deviceId }, 201);
  }

  const user = await auth(req);
  if (!user) return fail("authentication required", 401);

  if (path === "/api/v1/me" && req.method === "GET") return json({ user, workspaces: userWorkspaces(user.id) });
  if (path === "/api/v1/dashboard" && req.method === "GET") return json(dashboard(user, url.searchParams.get("workspace")));

  if (path === "/api/v1/passkeys" && req.method === "GET") {
    const human = await cookieUser(req); if (!human || human.id !== user.id) return fail("browser session required", 401);
    const passkeys = q<any>(`SELECT id,created_at,last_used FROM passkeys WHERE user_id=? ORDER BY created_at`).all(user.id);
    return json({ passkeys });
  }
  if (path === "/api/v1/passkeys/options" && req.method === "POST") {
    const human = await cookieUser(req); if (!human || human.id !== user.id) return fail("browser session required", 401);
    return json(await registrationCeremony("add-passkey", user.id, user.name), 201);
  }
  if (path === "/api/v1/passkeys/verify" && req.method === "POST") {
    const human = await cookieUser(req); if (!human || human.id !== user.id) return fail("browser session required", 401);
    const input = await body(req);
    const ceremony = takeCeremony(input.ceremonyId, "add-passkey");
    if (!ceremony || ceremony.user_id !== user.id) return fail("registration expired", 410);
    let credential;
    try { credential = await verifyNewPasskey(ceremony, input.response); }
    catch { return fail("passkey verification failed", 401); }
    insertPasskey(user.id, credential);
    return json({ ok: true }, 201);
  }

  let m = path.match(/^\/api\/v1\/passkeys\/([^/]+)$/);
  if (m && req.method === "DELETE") {
    const human = await cookieUser(req); if (!human || human.id !== user.id) return fail("browser session required", 401);
    const removed = q(`DELETE FROM passkeys WHERE id=? AND user_id=?`).run(m[1], user.id);
    if (removed.changes === 0) return fail("passkey not found", 404);
    return json({ ok: true });
  }

  if (path === "/api/v1/workspaces" && req.method === "POST") {
    const input = await body(req); const name = String(input.name || "").trim();
    if (!name) return fail("workspace name required");
    const workspaceId = id(), fleetId = id(), t = now();
    db.transaction(() => {
      q(`INSERT INTO workspaces VALUES(?,?,?,?)`).run(workspaceId, name.slice(0, 120), user.id, t);
      q(`INSERT INTO workspace_members VALUES(?,?,?,?)`).run(workspaceId, user.id, "owner", t);
      q(`INSERT INTO fleets VALUES(?,?,?,?)`).run(fleetId, workspaceId, "Default", t);
    })();
    logEvent("workspace.created", workspaceId, user.id, null, { name });
    return json({ id: workspaceId }, 201);
  }

  m = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/fleets$/);
  if (m && req.method === "POST") {
    const role = roleFor(user.id, m[1]); if (!canWrite(role)) return fail("forbidden", 403);
    const input = await body(req); const name = String(input.name || "").trim(); if (!name) return fail("fleet name required");
    const fleetId = id(); q(`INSERT INTO fleets VALUES(?,?,?,?)`).run(fleetId, m[1], name.slice(0, 120), now());
    logEvent("fleet.created", m[1], user.id, null, { fleetId, name });
    return json({ id: fleetId }, 201);
  }

  m = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/invites$/);
  if (m && req.method === "POST") {
    if (roleFor(user.id, m[1]) !== "owner") return fail("owner required", 403);
    const input = await body(req); const inviteRole = input.role === "viewer" ? "viewer" : "member";
    const token = opaque("invite"), inviteId = id(), t = now();
    q(`INSERT INTO workspace_invites VALUES(?,?,?,?,?,?,?,NULL)`).run(inviteId, m[1], sha(token), inviteRole, user.id, t, t + TOKEN_TTL);
    return json({ token, expiresAt: t + TOKEN_TTL }, 201);
  }

  if (path === "/api/v1/workspaces/join" && req.method === "POST") {
    const input = await body(req); const token = String(input.token || "").trim();
    const invite = q<any>(`SELECT * FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?`).get(sha(token), now());
    if (!invite) return fail("invalid or expired invite", 401);
    db.transaction(() => {
      q(`INSERT OR IGNORE INTO workspace_members VALUES(?,?,?,?)`).run(invite.workspace_id, user.id, invite.role, now());
      q(`UPDATE workspace_invites SET used_at=? WHERE id=?`).run(now(), invite.id);
    })();
    logEvent("member.joined", invite.workspace_id, user.id, null, { role: invite.role });
    return json({ workspaceId: invite.workspace_id });
  }

  m = path.match(/^\/api\/v1\/fleets\/([^/]+)\/enrollments$/);
  if (m && req.method === "POST") {
    const fleet = q<any>(`SELECT workspace_id FROM fleets WHERE id=?`).get(m[1]); if (!fleet) return fail("fleet not found", 404);
    if (!canWrite(roleFor(user.id, fleet.workspace_id))) return fail("forbidden", 403);
    const token = opaque("enroll"), enrollmentId = id(), t = now();
    q(`INSERT INTO enrollment_tokens VALUES(?,?,?,?,?,?,NULL)`).run(enrollmentId, m[1], sha(token), user.id, t, t + TOKEN_TTL);
    return json({ token, expiresAt: t + TOKEN_TTL, install: `curl -fsSL ${PUBLIC_URL}/install.sh | sh -s -- ${token}` }, 201);
  }

  m = path.match(/^\/api\/v1\/fleets\/([^/]+)\/devices\/([^/]+)$/);
  if (m && req.method === "POST") {
    const fleet = q<any>(`SELECT workspace_id FROM fleets WHERE id=?`).get(m[1]); if (!fleet) return fail("fleet not found", 404);
    if (!canWrite(roleFor(user.id, fleet.workspace_id)) || !deviceRole(user.id, m[2])) return fail("forbidden", 403);
    const deviceWorkspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id WHERE fd.device_id=? LIMIT 1`).get(m[2]);
    if (!deviceWorkspace || deviceWorkspace.workspace_id !== fleet.workspace_id) return fail("devices cannot cross workspaces", 409);
    q(`INSERT OR IGNORE INTO fleet_devices VALUES(?,?,?,?)`).run(m[1], m[2], JSON.stringify(["shell"]), now());
    return json({ ok: true }, 201);
  }

  m = path.match(/^\/api\/v1\/devices\/([^/]+)\/sessions$/);
  if (m && req.method === "POST") {
    const role = deviceRole(user.id, m[1]); if (!canWrite(role)) return fail("forbidden", 403);
    if (!devicePermission(user.id, m[1], "shell")) return fail("shell capability not permitted", 403);
    const sessionId = id(); q(`INSERT INTO sessions(id,device_id,user_id,type,status,created_at) VALUES(?,?,?,?,?,?)`).run(sessionId, m[1], user.id, "shell", "active", now());
    return json({ id: sessionId }, 201);
  }

  m = path.match(/^\/api\/v1\/sessions\/([^/]+)\/jobs$/);
  if (m && req.method === "POST") {
    const access = sessionAccess(user.id, m[1]); if (!access || !canWrite(access.role)) return fail("forbidden", 403);
    const input = await body(req); const command = String(input.command || "").trim(); if (!command) return fail("command required");
    if (command.length > 8192) return fail("command too long");
    const jobId = id(), t = now();
    q(`INSERT INTO jobs(id,device_id,session_id,type,payload,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)`).run(jobId, access.deviceId, m[1], "shell", JSON.stringify({ command }), "pending", user.id, t);
    dispatchJob(jobId, access.deviceId, command);
    const workspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id WHERE fd.device_id=? LIMIT 1`).get(access.deviceId);
    logEvent("job.created", workspace?.workspace_id || null, user.id, access.deviceId, { jobId, command });
    return json({ id: jobId }, 201);
  }
  if (m && req.method === "GET") {
    const access = sessionAccess(user.id, m[1]); if (!access?.role) return fail("forbidden", 403);
    const jobs = q<any>(`SELECT id,type,payload,status,result,exit_code,created_at,started_at,completed_at FROM jobs WHERE session_id=? ORDER BY created_at`).all(m[1]).map((j: any) => ({ ...j, payload: JSON.parse(j.payload) }));
    return json({ jobs });
  }

  m = path.match(/^\/api\/v1\/sessions\/([^/]+)$/);
  if (m && req.method === "DELETE") {
    const access = sessionAccess(user.id, m[1]); if (!access || !canWrite(access.role)) return fail("forbidden", 403);
    q(`UPDATE sessions SET status='closed',closed_at=? WHERE id=?`).run(now(), m[1]);
    return json({ ok: true });
  }

  if (path === "/api/v1/tokens" && req.method === "POST") {
    const input = await body(req); const name = String(input.name || "API token").trim().slice(0, 80);
    const token = opaque("rly"), tokenId = id();
    q(`INSERT INTO api_tokens(id,user_id,name,token_hash,created_at) VALUES(?,?,?,?,?)`).run(tokenId, user.id, name, sha(token), now());
    return json({ id: tokenId, token }, 201);
  }

  return fail("not found", 404);
}

function dispatchJob(jobId: string, deviceId: string, command: string) {
  const ws = agents.get(deviceId);
  if (!ws) return false;
  try {
    ws.send(JSON.stringify({ type: "job", id: jobId, command }));
    q(`UPDATE jobs SET status='sent',started_at=? WHERE id=? AND status='pending'`).run(now(), jobId);
    return true;
  } catch {
    return false;
  }
}

function sendPending(deviceId: string) {
  const rows = q<any>(`SELECT id,payload FROM jobs WHERE device_id=? AND status='pending' ORDER BY created_at LIMIT 20`).all(deviceId);
  for (const row of rows) dispatchJob(row.id, deviceId, JSON.parse(row.payload).command);
}

function verifyAgent(url: URL): string | null {
  const deviceId = url.searchParams.get("device") || "";
  const ts = url.searchParams.get("ts") || "";
  const sig = url.searchParams.get("sig") || "";
  const seconds = Number(ts);
  if (!deviceId || !Number.isFinite(seconds) || Math.abs(Date.now() / 1000 - seconds) > 60 || !sig) return null;
  const row = q<any>(`SELECT public_key FROM devices WHERE id=?`).get(deviceId);
  if (!row) return null;
  try {
    const key = createPublicKey(row.public_key);
    const ok = verifySignature(null, Buffer.from(`relay:${deviceId}:${ts}`), key, Buffer.from(sig, "base64url"));
    return ok ? deviceId : null;
  } catch { return null; }
}

function contentType(path: string) {
  return ({ ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".png": "image/png", ".sh": "text/plain; charset=utf-8" } as Record<string,string>)[extname(path)] || "application/octet-stream";
}

async function staticResponse(pathname: string) {
  let relative = pathname === "/" ? "index.html" : pathname.replace(/^\/+/, "");
  if (relative.includes("..")) return fail("not found", 404);
  const path = join(import.meta.dir, "public", relative);
  if (!existsSync(path)) return fail("not found", 404);
  const immutable = pathname.startsWith("/downloads/");
  const headers: Record<string, string> = {
    "content-type": contentType(path),
    "cache-control": immutable ? "public, max-age=3600" : "no-store, max-age=0",
  };
  if (!immutable) {
    headers["cdn-cache-control"] = "no-store";
    headers["cloudflare-cdn-cache-control"] = "no-store";
  }
  return new Response(Bun.file(path), { headers });
}

const server = Bun.serve<AgentData>({
  port: PORT,
  hostname: "0.0.0.0",
  async fetch(req, server) {
    const url = new URL(req.url);
    try {
      if (req.method === "GET" && url.pathname === "/" && url.searchParams.has("setup")) {
        if ((q<any>(`SELECT count(*) count FROM users`).get()?.count || 0) > 0) return Response.redirect(PUBLIC_URL + "/", 303);
        const token = url.searchParams.get("setup") || "";
        if (!SETUP_TOKEN || sha(token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
        return new Response(null, {
          status: 303,
          headers: {
            location: "/",
            "set-cookie": setupCookie(token),
            "cache-control": "no-store",
          },
        });
      }
      if (url.pathname === "/api/v1/agent/ws") {
        const deviceId = verifyAgent(url);
        if (!deviceId) return fail("invalid agent signature", 401);
        if (server.upgrade(req, { data: { kind: "agent", deviceId } })) return undefined;
        return fail("upgrade failed", 400);
      }
      if (url.pathname.startsWith("/api/v1/")) return await handleAPI(req, url);
      if (url.pathname === "/healthz") return new Response("ok");
      if (url.pathname === "/robots.txt") return new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } });
      return await staticResponse(url.pathname);
    } catch (error: any) {
      console.error(error);
      return fail(error?.message || "internal error", 500);
    }
  },
  websocket: {
    open(ws) {
      const { deviceId } = ws.data;
      const previous = agents.get(deviceId);
      if (previous && previous !== ws) previous.close(1012, "replaced");
      agents.set(deviceId, ws);
      q(`UPDATE devices SET last_seen=? WHERE id=?`).run(now(), deviceId);
      const workspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id WHERE fd.device_id=? LIMIT 1`).get(deviceId);
      logEvent("device.online", workspace?.workspace_id || null, null, deviceId);
      sendPending(deviceId);
    },
    message(ws, raw) {
      try {
        const msg = JSON.parse(typeof raw === "string" ? raw : Buffer.from(raw as any).toString("utf8"));
        const deviceId = ws.data.deviceId;
        if (msg.type === "heartbeat") {
          q(`UPDATE devices SET last_seen=? WHERE id=?`).run(now(), deviceId);
          return;
        }
        if (msg.type === "result" && msg.id) {
          const job = q<any>(`SELECT id,session_id FROM jobs WHERE id=? AND device_id=?`).get(String(msg.id), deviceId);
          if (!job) return;
          const output = String(msg.output || "").slice(0, 1024 * 1024);
          const exitCode = Number.isInteger(msg.exitCode) ? msg.exitCode : -1;
          q(`UPDATE jobs SET status=?,result=?,exit_code=?,completed_at=? WHERE id=?`).run(exitCode === 0 ? "completed" : "failed", output, exitCode, now(), job.id);
          q(`UPDATE devices SET last_seen=? WHERE id=?`).run(now(), deviceId);
        }
      } catch (error) { console.error("agent message", error); }
    },
    close(ws) {
      const { deviceId } = ws.data;
      if (agents.get(deviceId) === ws) {
        agents.delete(deviceId);
        const workspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id WHERE fd.device_id=? LIMIT 1`).get(deviceId);
        logEvent("device.offline", workspace?.workspace_id || null, null, deviceId);
      }
    }
  }
});

setInterval(() => {
  q(`DELETE FROM auth_sessions WHERE expires_at<?`).run(now());
  q(`DELETE FROM workspace_invites WHERE expires_at<? AND used_at IS NULL`).run(now());
  q(`DELETE FROM enrollment_tokens WHERE expires_at<? AND used_at IS NULL`).run(now());
  q(`DELETE FROM webauthn_challenges WHERE expires_at<?`).run(now());
}, 60_000).unref();

console.log(`Relay ${PUBLIC_URL} listening on :${server.port}; database ${DB_PATH}`);
