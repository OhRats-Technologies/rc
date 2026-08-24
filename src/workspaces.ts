import { PUBLIC_URL, TOKEN_TTL } from "./config";
import { canWrite, logEvent, roleFor, User, userWorkspaces } from "./core";
import { db, id, now, opaque, q, sha } from "./db";
import { disconnectDevice, isOnline } from "./gateway";
import { HttpError } from "./errors";
import type { DeviceView } from "./devices";

export type WorkspaceView = { id: string; name: string; role: "owner" | "member" | "viewer"; created_at: number };
export type ActivityView = { id: number; kind: string; detail: Record<string, unknown>; device_id: string | null; created_at: number };

export function workspaceFor(user: User, workspaceId: string): WorkspaceView | null {
  const role = roleFor(user.id, workspaceId);
  if (!role) return null;
  const row = q<{ id: string; name: string; created_at: number }>("SELECT id,name,created_at FROM workspaces WHERE id=?").get(workspaceId);
  return row ? { ...row, role } : null;
}

export function workspaceDevices(workspaceId: string): DeviceView[] {
  type Row = Omit<DeviceView, "capabilities" | "online" | "role"> & { capabilities: string };
  return q<Row>(`SELECT d.id,d.workspace_id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,
    d.last_seen,d.created_at,w.name workspace_name FROM devices d JOIN workspaces w ON w.id=d.workspace_id
    WHERE d.workspace_id=? ORDER BY d.name`).all(workspaceId)
    .map(row => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") as string[] }));
}

export function workspaceDetail(user: User, workspaceId: string) {
  const workspace = workspaceFor(user, workspaceId);
  return workspace ? { workspace, devices: workspaceDevices(workspaceId) } : null;
}

export function createWorkspace(user: User, value: unknown) {
  const name = String(value || "").trim().slice(0, 120);
  if (!name) throw new HttpError(400, "workspace name required");
  const workspaceId = id(), t = now();
  db.transaction(() => {
    q("INSERT INTO workspaces VALUES(?,?,?,?)").run(workspaceId, name, user.id, t);
    q("INSERT INTO workspace_members VALUES(?,?,?,?)").run(workspaceId, user.id, "owner", t);
  })();
  logEvent("workspace.created", workspaceId, user.id, null, { name });
  return { id: workspaceId };
}

export function deleteWorkspace(user: User, workspaceId: string) {
  if (roleFor(user.id, workspaceId) !== "owner") throw new HttpError(403, "owner required");
  const deviceIds = q<{ id: string }>("SELECT id FROM devices WHERE workspace_id=?").all(workspaceId).map(row => row.id);
  for (const deviceId of deviceIds) disconnectDevice(deviceId);
  const removed = q("DELETE FROM workspaces WHERE id=?").run(workspaceId);
  if (!removed.changes) throw new HttpError(404, "workspace not found");
}

export function createInvite(user: User, workspaceId: string, role: unknown) {
  if (roleFor(user.id, workspaceId) !== "owner") throw new HttpError(403, "owner required");
  const inviteRole = role === "viewer" ? "viewer" : "member";
  const token = opaque("invite"), inviteId = id(), t = now();
  q("INSERT INTO workspace_invites VALUES(?,?,?,?,?,?,?,NULL)")
    .run(inviteId, workspaceId, sha(token), inviteRole, user.id, t, t + TOKEN_TTL);
  return { token, url: `${PUBLIC_URL}/?invite=${encodeURIComponent(token)}`, expiresAt: t + TOKEN_TTL };
}

export function invitePreview(value: unknown) {
  const token = String(value || "").trim();
  if (!token) return null;
  return q<{ workspaceId: string; workspaceName: string; role: "member" | "viewer" }>(`SELECT i.workspace_id workspaceId,
    w.name workspaceName,i.role FROM workspace_invites i JOIN workspaces w ON w.id=i.workspace_id
    WHERE i.token_hash=? AND i.used_at IS NULL AND i.expires_at>?`).get(sha(token), now()) || null;
}

export function joinWorkspace(user: User, value: unknown) {
  const token = String(value || "").trim();
  const invite = q<any>("SELECT * FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?").get(sha(token), now());
  if (!invite) throw new HttpError(401, "invalid or expired invite");
  db.transaction(() => {
    q("INSERT OR IGNORE INTO workspace_members VALUES(?,?,?,?)").run(invite.workspace_id, user.id, invite.role, now());
    q("UPDATE workspace_invites SET used_at=? WHERE id=?").run(now(), invite.id);
  })();
  logEvent("member.joined", invite.workspace_id, user.id, null, { role: invite.role });
  return { workspaceId: String(invite.workspace_id) };
}

export function createEnrollment(user: User, workspaceId: string) {
  if (!canWrite(roleFor(user.id, workspaceId))) throw new HttpError(403, "forbidden");
  const token = opaque("enroll"), enrollmentId = id(), t = now();
  q("INSERT INTO enrollment_tokens VALUES(?,?,?,?,?,?,NULL)").run(enrollmentId, workspaceId, sha(token), user.id, t, t + TOKEN_TTL);
  return { token, expiresAt: t + TOKEN_TTL, install: `curl -fsSL ${PUBLIC_URL}/install.sh | sh -s -- ${token}` };
}

export function workspaceActivity(user: User, workspaceId: string): ActivityView[] {
  if (!roleFor(user.id, workspaceId)) throw new HttpError(403, "forbidden");
  return q<Omit<ActivityView, "detail"> & { detail: string }>(
    "SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 100"
  ).all(workspaceId).map(event => ({ ...event, detail: JSON.parse(event.detail || "{}") as Record<string, unknown> }));
}
