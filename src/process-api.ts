import { canOperate, deviceRole, logEvent, type Role } from "./core";
import { id, now, q } from "./db";
import { isOnline } from "./gateway";
import { markProcessLost, processJSON, processRow, workspaceForDevice, type ProcessOrigin } from "./process-store";
import { HttpError } from "./errors";
import { publishEvent } from "./events";

export type AllocateProcessInput = {
  deviceId: string; origin?: Extract<ProcessOrigin, "browser" | "cli" | "api">; terminal?: boolean;
};

function processAccess(userId: string, processId: string) {
  const process = processRow(processId);
  if (!process) throw new HttpError(404, "process not found");
  const role = deviceRole(userId, process.device_id);
  if (!role) throw new HttpError(403, "forbidden");
  return { process, role };
}

function canControl(role: Role, userId: string, process: any) {
  return role === "owner" || (role === "operator" && process.created_by === userId);
}

export function allocateProcess(userId: string, input: AllocateProcessInput) {
  const deviceId = input.deviceId, role = deviceRole(userId, deviceId);
  if (!canOperate(role)) throw new HttpError(403, "operator required");
  if (!isOnline(deviceId)) throw new HttpError(409, "device is offline");
  const processId = id(), t = now(), origin = input.origin || "api";
  q(`INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at)
    VALUES(?,?,?,'starting',?,?,?)`).run(processId, deviceId, origin, input.terminal ? 1 : 0, userId, t);
  logEvent("process.created", workspaceForDevice(deviceId), userId, deviceId, { processId, origin, terminal: Boolean(input.terminal) });
  return { processId };
}

export function listProcesses(userId: string, deviceId: string) {
  if (!canOperate(deviceRole(userId, deviceId))) throw new HttpError(403, "operator required");
  return q<any>(`SELECT p.*,u.name created_by_name FROM processes p JOIN users u ON u.id=p.created_by
    WHERE p.device_id=? ORDER BY p.created_at DESC LIMIT 100`).all(deviceId).map(processJSON);
}

export function getProcess(userId: string, processId: string) {
  const allowed = processAccess(userId, processId);
  if (!canOperate(allowed.role)) throw new HttpError(403, "operator required");
  const row = q<any>("SELECT p.*,u.name created_by_name FROM processes p JOIN users u ON u.id=p.created_by WHERE p.id=?").get(processId);
  return processJSON(row || allowed.process);
}

setInterval(() => {
  const cutoff = now() - 60_000;
  const stale = q<{ id: string; device_id: string }>(
    "SELECT id,device_id FROM processes WHERE origin IN ('browser','cli','api','control') AND status='starting' AND created_at<?"
  ).all(cutoff);
  for (const process of stale) {
    const error = "encrypted process was not acknowledged by the RC Node";
    markProcessLost(process.id, error);
    publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(process.device_id), deviceId: process.device_id, processId: process.id, detail: { error } });
  }
}, 30_000).unref();
