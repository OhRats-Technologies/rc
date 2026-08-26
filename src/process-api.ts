import { canOperate, deviceRole, logEvent, type Role } from "./core";
import { id, now, q } from "./db";
import { isOnline } from "./gateway";
import { markProcessLost, processJSON, processRow, workspaceForDevice } from "./process-store";
import { HttpError } from "./errors";
import { publishEvent } from "./events";

export type StartProcessInput = { deviceId: string; command: string; cwd?: string; cols: number; rows: number };
export type AllocateProcessInput = { deviceId: string; terminal?: boolean; cols: number; rows: number };
export type ProcessInput = { processId: string; data: string };
export type ProcessResize = { processId: string; cols: number; rows: number };
export type ProcessSignal = { processId: string; signal: "INT" | "TERM" | "KILL" };

function boundedSize(value: unknown, fallback: number) {
  const number = Number(value);
  return Number.isInteger(number) && number >= 2 && number <= 500 ? number : fallback;
}

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

export function startProcess(userId: string, input: StartProcessInput) {
  void userId; void input;
  throw new HttpError(426, "end-to-end control client required");
}

export function allocateProcess(userId: string, input: AllocateProcessInput) {
  const deviceId = input.deviceId, role = deviceRole(userId, deviceId);
  if (!canOperate(role)) throw new HttpError(403, "operator required");
  if (!isOnline(deviceId)) throw new HttpError(409, "device is offline");
  const cols = boundedSize(input.cols, 80), rows = boundedSize(input.rows, 24), processId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,command,cwd,status,encrypted,terminal,cols,rows,created_by,created_at)
    VALUES(?,?,?,NULL,'starting',1,?,?,?,?,?)`).run(processId, deviceId, "[encrypted]", input.terminal ? 1 : 0, cols, rows, userId, t);
  logEvent("process.created", workspaceForDevice(deviceId), userId, deviceId, { processId, encrypted: true });
  return { processId };
}

export function inputProcess(userId: string, input: ProcessInput) {
  void userId; void input; throw new HttpError(426, "end-to-end control client required");
}

export function resizeRemoteProcess(userId: string, input: ProcessResize) {
  void userId; void input; throw new HttpError(426, "end-to-end control client required");
}

export function signalProcess(userId: string, input: ProcessSignal) {
  void userId; void input; throw new HttpError(426, "end-to-end control client required");
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
    "SELECT id,device_id FROM processes WHERE encrypted=1 AND status='starting' AND created_at<?"
  ).all(cutoff);
  for (const process of stale) {
    markProcessLost(process.id, "encrypted process was not acknowledged by the RC Node");
    publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(process.device_id), deviceId: process.device_id,
      processId: process.id, detail: { error: "encrypted process was not acknowledged by the RC Node" } });
  }
}, 30_000).unref();
