import { q, now } from "./db";
import { publishEvent } from "./events";

export type User = { id: string; name: string };
export type Role = "owner" | "member" | "viewer";

export function roleFor(userId: string, workspaceId: string): Role | null {
  return q<any>("SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?").get(workspaceId, userId)?.role || null;
}
export function canWrite(role: Role | null) { return role === "owner" || role === "member"; }
export function userWorkspaces(userId: string) {
  return q<any>(`SELECT w.id,w.name,wm.role,w.created_at FROM workspaces w
    JOIN workspace_members wm ON wm.workspace_id=w.id WHERE wm.user_id=? ORDER BY w.created_at`).all(userId);
}
export function deviceRole(userId: string, deviceId: string): Role | null {
  return q<any>(`SELECT wm.role FROM workspace_members wm JOIN fleets f ON f.workspace_id=wm.workspace_id
    JOIN fleet_devices fd ON fd.fleet_id=f.id WHERE wm.user_id=? AND fd.device_id=? LIMIT 1`).get(userId, deviceId)?.role || null;
}
export function devicePermission(userId: string, deviceId: string, capability: string) {
  const rows = q<any>(`SELECT fd.permissions FROM workspace_members wm JOIN fleets f ON f.workspace_id=wm.workspace_id
    JOIN fleet_devices fd ON fd.fleet_id=f.id WHERE wm.user_id=? AND fd.device_id=?`).all(userId, deviceId);
  return rows.some((row: any) => { try { return JSON.parse(row.permissions || "[]").includes(capability); } catch { return false; } });
}
export function sessionAccess(userId: string, sessionId: string) {
  const row = q<any>("SELECT device_id,status FROM sessions WHERE id=?").get(sessionId);
  return row?.status === "active" ? { deviceId: row.device_id, role: deviceRole(userId, row.device_id) } : null;
}
export function logEvent(kind: string, workspaceId: string | null, userId: string | null, deviceId: string | null, detail: unknown = {}) {
  const eventDetail = deviceId && detail && typeof detail === "object" && !Array.isArray(detail)
    ? { ...(detail as Record<string, unknown>), deviceId }
    : detail;
  q("INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES(?,?,?,?,?,?)")
    .run(workspaceId, userId, deviceId, kind, JSON.stringify(eventDetail), now());
  publishEvent({ kind, workspaceId, deviceId, audit: true, detail: eventDetail });
}
