import { PUBLIC_URL, TOKEN_TTL } from "./config";
import { canWrite, logEvent, roleFor, User, userWorkspaces } from "./core";
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
  return q<any>(`SELECT id,name,hostname,platform,arch,agent_version,capabilities,last_seen,created_at
    FROM devices WHERE workspace_id=? ORDER BY name`).all(workspaceId)
    .map((row: any) => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") }));
}

function detail(user: User, workspaceId: string) {
  const workspace = workspaceFor(user, workspaceId);
  return workspace ? { workspace, devices: workspaceDevices(workspaceId) } : null;
}

async function createWorkspace(req: Request, user: User) {
  const input = await body(req), name = String(input.name || "").trim().slice(0, 120);
  if (!name) return fail("workspace name required");
  const workspaceId = id(), t = now();
  db.transaction(() => {
    q("INSERT INTO workspaces VALUES(?,?,?,?)").run(workspaceId, name, user.id, t);
    q("INSERT INTO workspace_members VALUES(?,?,?,?)").run(workspaceId, user.id, "owner", t);
  })();
  logEvent("workspace.created", workspaceId, user.id, null, { name });
  return json({ id: workspaceId }, 201);
}

function deleteWorkspace(user: User, workspaceId: string) {
  if (roleFor(user.id, workspaceId) !== "owner") return fail("owner required", 403);
  const deviceIds = q<any>("SELECT id FROM devices WHERE workspace_id=?").all(workspaceId).map((row: any) => row.id);
  for (const deviceId of deviceIds) disconnectDevice(deviceId);
  const removed = q("DELETE FROM workspaces WHERE id=?").run(workspaceId);
  return removed.changes ? json({ ok: true }) : fail("workspace not found", 404);
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

function enrollment(user: User, workspaceId: string) {
  if (!canWrite(roleFor(user.id, workspaceId))) return fail("forbidden", 403);
  const token = opaque("enroll"), enrollmentId = id(), t = now();
  q("INSERT INTO enrollment_tokens VALUES(?,?,?,?,?,?,NULL)").run(enrollmentId, workspaceId, sha(token), user.id, t, t + TOKEN_TTL);
  return json({ token, expiresAt: t + TOKEN_TTL, install: `curl -fsSL ${PUBLIC_URL}/install.sh | sh -s -- ${token}` }, 201);
}

export async function handleWorkspaces(req: Request, path: string, user: User): Promise<Response | null> {
  if (path === "/api/v1/workspaces" && req.method === "GET") return json({ workspaces: userWorkspaces(user.id) });
  if (path === "/api/v1/workspaces" && req.method === "POST") return createWorkspace(req, user);
  if (path === "/api/v1/workspaces/join" && req.method === "POST") return join(req, user);
  let match = path.match(/^\/api\/v1\/workspaces\/([^/]+)$/);
  if (match && req.method === "GET") { const data = detail(user, match[1]); return data ? json(data) : fail("workspace not found", 404); }
  if (match && req.method === "DELETE") return deleteWorkspace(user, match[1]);
  match = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/activity$/);
  if (match && req.method === "GET") {
    if (!roleFor(user.id, match[1])) return fail("forbidden", 403);
    const events = q<any>("SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 100")
      .all(match[1]).map((event: any) => ({ ...event, detail: JSON.parse(event.detail || "{}") }));
    return json({ events });
  }
  match = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/invites$/);
  if (match && req.method === "POST") return invite(req, user, match[1]);
  match = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/enrollments$/);
  if (match && req.method === "POST") return enrollment(user, match[1]);
  return null;
}
