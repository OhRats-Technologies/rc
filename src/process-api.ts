import { canWrite, deviceRole, logEvent } from "./core";
import { id, now, q } from "./db";
import { dispatchProcessStart, isOnline, sendProcessControl } from "./gateway";
import { processJSON, processRow, resizeProcess, workspaceForDevice } from "./process-store";
import { HttpError } from "./errors";

export type StartProcessInput = { deviceId: string; command: string; cwd?: string; cols: number; rows: number };
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

export function startProcess(userId: string, input: StartProcessInput) {
  const deviceId = input.deviceId, role = deviceRole(userId, deviceId);
  if (!canWrite(role)) throw new HttpError(403, "forbidden");
  if (!isOnline(deviceId)) throw new HttpError(409, "device is offline");
  const command = input.command.trim(), cwd = String(input.cwd || "").trim().slice(0, 4096) || null;
  if (!command) throw new HttpError(400, "command required");
  if (command.length > 8192) throw new HttpError(400, "command too long");
  const cols = boundedSize(input.cols, 80), rows = boundedSize(input.rows, 24), processId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,command,cwd,status,cols,rows,created_by,created_at)
    VALUES(?,?,?,?,'starting',?,?,?,?)`).run(processId, deviceId, command, cwd, cols, rows, userId, t);
  if (!dispatchProcessStart(processId, deviceId, command, cwd, cols, rows)) {
    q("DELETE FROM processes WHERE id=?").run(processId);
    throw new HttpError(409, "device connection changed");
  }
  logEvent("process.created", workspaceForDevice(deviceId), userId, deviceId, { processId, command, cwd });
  return { processId };
}

export function inputProcess(userId: string, input: ProcessInput) {
  const processId = input.processId, allowed = processAccess(userId, processId);
  if (!["starting", "running"].includes(allowed.process.status)) throw new HttpError(409, "process is not running");
  if (!isOnline(allowed.process.device_id)) throw new HttpError(409, "device is offline");
  const data = input.data;
  if (!data || data.length > 64 * 1024) throw new HttpError(400, "input must be 1-65536 characters");
  if (!sendProcessControl(allowed.process.device_id, { type: "process.input", id: processId, input: data })) throw new HttpError(409, "device connection changed");
  return { ok: true };
}

export function resizeRemoteProcess(userId: string, input: ProcessResize) {
  const processId = input.processId, allowed = processAccess(userId, processId);
  if (!['starting','running'].includes(allowed.process.status)) return { ok: true };
  const cols = boundedSize(input.cols, Number(allowed.process.cols || 80));
  const rows = boundedSize(input.rows, Number(allowed.process.rows || 24));
  if (!sendProcessControl(allowed.process.device_id, { type: "process.resize", id: processId, cols, rows })) throw new HttpError(409, "device connection changed");
  resizeProcess(processId, cols, rows);
  return { ok: true };
}

export function signalProcess(userId: string, input: ProcessSignal) {
  const processId = input.processId, allowed = processAccess(userId, processId);
  if (!['starting','running'].includes(allowed.process.status)) return { ok: true };
  const signal = input.signal;
  if (!sendProcessControl(allowed.process.device_id, { type: "process.signal", id: processId, signal })) throw new HttpError(409, "device connection changed");
  return { ok: true };
}

export function listProcesses(userId: string, deviceId: string) {
  if (!deviceRole(userId, deviceId)) throw new HttpError(403, "forbidden");
  return q<any>("SELECT * FROM processes WHERE device_id=? ORDER BY created_at DESC LIMIT 100").all(deviceId).map(processJSON);
}

export function getProcess(userId: string, processId: string) {
  return processJSON(processAccess(userId, processId).process);
}
