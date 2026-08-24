import { PUBLIC_URL, TOKEN_TTL } from "./config";
import { canWrite, logEvent, roleFor, User, userWorkspaces, deviceRole } from "./core";
import { db, id, now, opaque, q, sha } from "./db";
import { disconnectDevice, isOnline } from "./gateway";
import { body, fail, json } from "./http-utils";

function workspaceFor(user: User, workspaceId: string) {
  const role = roleFor(user.id, workspaceId);
  if (!role) return null;
  const row = q<any>("SELECT id,name,created_at FROM workspaces WHERE id=?").get(workspaceId);
  return row ? { ...row, role } : null;
}

function workspaceDevices(workspaceId: string) {
  return q<any>(`SELECT d.id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,d.last_seen,d.created_at,
    group_concat(f.name, ', ') fleets FROM devices d JOIN fleet_devices fd ON fd.device_id=d.id
    JOIN fleets f ON f.id=fd.fleet_id WHERE f.workspace_id=? GROUP BY d.id ORDER BY d.name`).all(workspaceId)
    .map((row: any) => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") }));
}

function cleanupOrphans(deviceIds: string[]) {
  for (const deviceId of deviceIds) {
    if (q<any>("SELECT 1 ok FROM fleet_devices WHERE device_id=? LIMIT 1").get(deviceId)) continue;
    disconnectDevice(deviceId);
    q("DELETE FROM devices WHERE id=?").run(deviceId);
  }
}

function detail(user: User, workspaceId: string) {
  const workspace = workspaceFor(user, workspaceId);
  if (!workspace) return null;
  const fleets = q<any>(`SELECT f.id,f.name,f.created_at,count(fd.device_id) device_count FROM fleets f
    LEFT JOIN fleet_devices fd ON fd.fleet_id=f.id WHERE f.workspace_id=? GROUP BY f.id ORDER BY f.created_at`).all(workspaceId);
  return { workspace, fleets, devices: workspaceDevices(workspaceId) };
}

async function createWorkspace(req: Request, user: User) {
  const input = await body(req), name = String(input.name || "").trim().slice(0, 120);
  if (!name) return fail("workspace name required");
  const workspaceId = id(), fleetId = id(), t = now();
  db.transaction(() => {
    q("INSERT INTO workspaces VALUES(?,?,?,?)").run(workspaceId, name, user.id, t);
    q("INSERT INTO workspace_members VALUES(?,?,?,?)").run(workspaceId, user.id, "owner", t);
    q("INSERT INTO fleets VALUES(?,?,?,?)").run(fleetId, workspaceId, "Default", t);
  })();
  logEvent("workspace.created", workspaceId, user.id, null, { name });
  return json({ id: workspaceId }, 201);
}

function deleteWorkspace(user: User, workspaceId: string) {
  if (roleFor(user.id, workspaceId) !== "owner") return fail("owner required", 403);
  const deviceIds = q<any>(`SELECT DISTINCT fd.device_id id FROM fleets f JOIN fleet_devices fd ON fd.fleet_id=f.id
    WHERE f.workspace_id=?`).all(workspaceId).map((row: any) => row.id);
  const removed = q("DELETE FROM workspaces WHERE id=?").run(workspaceId);
  if (!removed.changes) return fail("workspace not found", 404);
  cleanupOrphans(deviceIds);
  return json({ ok: true });
}

async function createFleet(req: Request, user: User, workspaceId: string) {
  if (!canWrite(roleFor(user.id, workspaceId))) return fail("forbidden", 403);
  const input = await body(req), name = String(input.name || "").trim().slice(0, 120);
  if (!name) return fail("fleet name required");
  const fleetId = id();
  q("INSERT INTO fleets VALUES(?,?,?,?)").run(fleetId, workspaceId, name, now());
  logEvent("fleet.created", workspaceId, user.id, null, { fleetId, name });
  return json({ id: fleetId }, 201);
}

function fleetDetail(user: User, fleetId: string) {
  const fleet = q<any>("SELECT id,workspace_id,name,created_at FROM fleets WHERE id=?").get(fleetId);
  if (!fleet || !roleFor(user.id, fleet.workspace_id)) return null;
  const devices = q<any>(`SELECT d.id,d.name,d.platform,d.arch,d.last_seen FROM devices d
    JOIN fleet_devices fd ON fd.device_id=d.id WHERE fd.fleet_id=? ORDER BY d.name`).all(fleetId)
    .map((row: any) => ({ ...row, online: isOnline(row.id) }));
  return { fleet, devices };
}

function deleteFleet(user: User, fleetId: string) {
  const fleet = q<any>("SELECT workspace_id FROM fleets WHERE id=?").get(fleetId);
  if (!fleet) return fail("fleet not found", 404);
  if (!canWrite(roleFor(user.id, fleet.workspace_id))) return fail("forbidden", 403);
  const deviceIds = q<any>("SELECT device_id id FROM fleet_devices WHERE fleet_id=?").all(fleetId).map((row: any) => row.id);
  q("DELETE FROM fleets WHERE id=?").run(fleetId);
  cleanupOrphans(deviceIds);
  logEvent("fleet.deleted", fleet.workspace_id, user.id, null, { fleetId });
  return json({ ok: true });
}

async function invite(req: Request, user: User, workspaceId: string) {
  if (roleFor(user.id, workspaceId) !== "owner") return fail("owner required", 403);
  const input = await body(req), inviteRole = input.role === "viewer" ? "viewer" : "member";
  const token = opaque("invite"), inviteId = id(), t = now();
  q("INSERT INTO workspace_invites VALUES(?,?,?,?,?,?,?,NULL)")
    .run(inviteId, workspaceId, sha(token), inviteRole, user.id, t, t + TOKEN_TTL);
  return json({ token, url: `${PUBLIC_URL}/?invite=${encodeURIComponent(token)}`, expiresAt: t + TOKEN_TTL }, 201);
}

async function join(req: Request, user: User) {
  const input = await body(req), token = String(input.token || "").trim();
  const invite = q<any>("SELECT * FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?").get(sha(token), now());
  if (!invite) return fail("invalid or expired invite", 401);
  db.transaction(() => {
    q("INSERT OR IGNORE INTO workspace_members VALUES(?,?,?,?)").run(invite.workspace_id, user.id, invite.role, now());
    q("UPDATE workspace_invites SET used_at=? WHERE id=?").run(now(), invite.id);
  })();
  logEvent("member.joined", invite.workspace_id, user.id, null, { role: invite.role });
  return json({ workspaceId: invite.workspace_id });
}

async function enrollment(user: User, fleetId: string) {
  const fleet = q<any>("SELECT workspace_id FROM fleets WHERE id=?").get(fleetId);
  if (!fleet) return fail("fleet not found", 404);
  if (!canWrite(roleFor(user.id, fleet.workspace_id))) return fail("forbidden", 403);
  const token = opaque("enroll"), enrollmentId = id(), t = now();
  q("INSERT INTO enrollment_tokens VALUES(?,?,?,?,?,?,NULL)").run(enrollmentId, fleetId, sha(token), user.id, t, t + TOKEN_TTL);
  return json({ token, expiresAt: t + TOKEN_TTL, install: `curl -fsSL ${PUBLIC_URL}/install.sh | sh -s -- ${token}` }, 201);
}

export async function handleWorkspaces(req: Request, path: string, user: User): Promise<Response | null> {
  if (path === "/api/v1/workspaces" && req.method === "GET") return json({ workspaces: userWorkspaces(user.id) });
  if (path === "/api/v1/workspaces" && req.method === "POST") return createWorkspace(req, user);
  if (path === "/api/v1/workspaces/join" && req.method === "POST") return join(req, user);
  let m = path.match(/^\/api\/v1\/workspaces\/([^/]+)$/);
  if (m && req.method === "GET") { const data = detail(user, m[1]); return data ? json(data) : fail("workspace not found", 404); }
  if (m && req.method === "DELETE") return deleteWorkspace(user, m[1]);
  m = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/activity$/);
  if (m && req.method === "GET") {
    if (!roleFor(user.id, m[1])) return fail("forbidden", 403);
    const events = q<any>("SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 100")
      .all(m[1]).map((e: any) => ({ ...e, detail: JSON.parse(e.detail || "{}") }));
    return json({ events });
  }
  m = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/fleets$/);
  if (m && req.method === "POST") return createFleet(req, user, m[1]);
  m = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/invites$/);
  if (m && req.method === "POST") return invite(req, user, m[1]);
  m = path.match(/^\/api\/v1\/fleets\/([^/]+)$/);
  if (m && req.method === "GET") { const data = fleetDetail(user, m[1]); return data ? json(data) : fail("fleet not found", 404); }
  if (m && req.method === "DELETE") return deleteFleet(user, m[1]);
  m = path.match(/^\/api\/v1\/fleets\/([^/]+)\/enrollments$/);
  if (m && req.method === "POST") return enrollment(user, m[1]);
  m = path.match(/^\/api\/v1\/fleets\/([^/]+)\/devices\/([^/]+)$/);
  if (m && req.method === "POST") {
    const fleet = q<any>("SELECT workspace_id FROM fleets WHERE id=?").get(m[1]);
    if (!fleet || !canWrite(roleFor(user.id, fleet.workspace_id)) || !deviceRole(user.id, m[2])) return fail("forbidden", 403);
    const current = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id
      WHERE fd.device_id=? LIMIT 1`).get(m[2]);
    if (!current || current.workspace_id !== fleet.workspace_id) return fail("devices cannot cross workspaces", 409);
    q("INSERT OR IGNORE INTO fleet_devices VALUES(?,?,?,?)").run(m[1], m[2], JSON.stringify(["shell"]), now());
    return json({ ok: true }, 201);
  }
  return null;
}
