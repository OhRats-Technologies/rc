import { canOperate, logEvent, roleFor, type User } from "./core";
import { id, now, q } from "./db";
import { HttpError } from "./errors";
import { startProcess } from "./process-api";

export type ActionView = {
  id: string; workspace_id: string; workspace_name: string; name: string; description: string;
  command: string; cwd: string | null; confirm: number; created_by: string; created_by_name: string;
  created_at: number; updated_at: number; role: "owner" | "operator" | "viewer";
};

function normalize(input: { name?: unknown; description?: unknown; command?: unknown; cwd?: unknown; confirm?: unknown }) {
  const name = String(input.name || "").trim().slice(0, 120);
  const description = String(input.description || "").trim().slice(0, 500);
  const command = String(input.command || "").trim().slice(0, 8192);
  const cwd = String(input.cwd || "").trim().slice(0, 4096) || null;
  const confirm = input.confirm === true || input.confirm === "1" || input.confirm === "on" ? 1 : 0;
  if (!name) throw new HttpError(400, "action name required");
  if (!command) throw new HttpError(400, "command required");
  return { name, description, command, cwd, confirm };
}

export function listActions(user: User, workspaceId?: string): ActionView[] {
  const where = workspaceId ? "AND a.workspace_id=?" : "";
  const args = workspaceId ? [user.id, workspaceId] : [user.id];
  return q<ActionView>(`SELECT a.*,w.name workspace_name,u.name created_by_name,wm.role FROM actions a
    JOIN workspaces w ON w.id=a.workspace_id JOIN users u ON u.id=a.created_by
    JOIN workspace_members wm ON wm.workspace_id=a.workspace_id AND wm.user_id=?
    WHERE 1=1 ${where} ORDER BY w.name,a.name`).all(...args);
}

export function getAction(user: User, actionId: string): ActionView | null {
  return q<ActionView>(`SELECT a.*,w.name workspace_name,u.name created_by_name,wm.role FROM actions a
    JOIN workspaces w ON w.id=a.workspace_id JOIN users u ON u.id=a.created_by
    JOIN workspace_members wm ON wm.workspace_id=a.workspace_id AND wm.user_id=? WHERE a.id=?`).get(user.id, actionId) || null;
}

export function createAction(user: User, workspaceId: string, input: Parameters<typeof normalize>[0]) {
  if (roleFor(user.id, workspaceId) !== "owner") throw new HttpError(403, "owner required");
  const value = normalize(input), actionId = id(), t = now();
  q(`INSERT INTO actions(id,workspace_id,name,description,command,cwd,confirm,created_by,created_at,updated_at)
    VALUES(?,?,?,?,?,?,?,?,?,?)`).run(actionId, workspaceId, value.name, value.description, value.command, value.cwd, value.confirm, user.id, t, t);
  logEvent("action.created", workspaceId, user.id, null, { actionId, name: value.name });
  return { id: actionId };
}

export function updateAction(user: User, actionId: string, input: Parameters<typeof normalize>[0]) {
  const action = getAction(user, actionId); if (!action) throw new HttpError(404, "action not found");
  if (action.role !== "owner") throw new HttpError(403, "owner required");
  const value = normalize(input);
  q("UPDATE actions SET name=?,description=?,command=?,cwd=?,confirm=?,updated_at=? WHERE id=?")
    .run(value.name, value.description, value.command, value.cwd, value.confirm, now(), actionId);
  logEvent("action.updated", action.workspace_id, user.id, null, { actionId, name: value.name });
}

export function deleteAction(user: User, actionId: string) {
  const action = getAction(user, actionId); if (!action) throw new HttpError(404, "action not found");
  if (action.role !== "owner") throw new HttpError(403, "owner required");
  q("DELETE FROM actions WHERE id=?").run(actionId);
  logEvent("action.deleted", action.workspace_id, user.id, null, { actionId, name: action.name });
}

export type ActionRunResult = { deviceId: string; deviceName: string; processId?: string; error?: string };

export function runAction(user: User, actionId: string, deviceIds: string[]): ActionRunResult[] {
  const action = getAction(user, actionId); if (!action) throw new HttpError(404, "action not found");
  if (!canOperate(action.role)) throw new HttpError(403, "operator required");
  const unique = [...new Set(deviceIds.map(String).filter(Boolean))].slice(0, 100);
  if (!unique.length) throw new HttpError(400, "select at least one device");
  const results = unique.map(deviceId => {
    const device = q<{ name: string; workspace_id: string }>("SELECT name,workspace_id FROM devices WHERE id=?").get(deviceId);
    if (!device || device.workspace_id !== action.workspace_id) return { deviceId, deviceName: device?.name || deviceId, error: "device is not in this workspace" };
    try {
      const { processId } = startProcess(user.id, { deviceId, command: action.command, cwd: action.cwd || undefined, cols: 100, rows: 30 });
      return { deviceId, deviceName: device.name, processId };
    } catch (error) { return { deviceId, deviceName: device.name, error: error instanceof Error ? error.message : "run failed" }; }
  });
  logEvent("action.run", action.workspace_id, user.id, null, { actionId, name: action.name, devices: results.map(result => ({ deviceId: result.deviceId, processId: result.processId || null, error: result.error || null })) });
  return results;
}
