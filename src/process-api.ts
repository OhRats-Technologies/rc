import { canWrite, deviceRole, logEvent } from "./core";
import { id, now, q } from "./db";
import { dispatchProcessStart, isOnline, sendProcessControl } from "./gateway";
import { fail, json } from "./http-utils";
import { processJSON, processRow, resizeProcess, workspaceForDevice } from "./process-store";

function boundedSize(value: unknown, fallback: number) {
  const number = Number(value);
  return Number.isInteger(number) && number >= 2 && number <= 500 ? number : fallback;
}

function processAccess(userId: string, processId: string) {
  const process = processRow(processId);
  if (!process) throw new Error("process not found");
  const role = deviceRole(userId, process.device_id);
  if (!role) throw new Error("forbidden");
  return { process, role };
}

export function startProcess(userId: string, input: any) {
  const deviceId = String(input.deviceId || ""), role = deviceRole(userId, deviceId);
  if (!canWrite(role)) throw new Error("forbidden");
  if (!isOnline(deviceId)) throw new Error("device is offline");
  const command = String(input.command || "").trim(), cwd = String(input.cwd || "").trim().slice(0, 4096) || null;
  if (!command) throw new Error("command required");
  if (command.length > 8192) throw new Error("command too long");
  const cols = boundedSize(input.cols, 80), rows = boundedSize(input.rows, 24), processId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,command,cwd,status,cols,rows,created_by,created_at)
    VALUES(?,?,?,?,'starting',?,?,?,?)`).run(processId, deviceId, command, cwd, cols, rows, userId, t);
  if (!dispatchProcessStart(processId, deviceId, command, cwd, cols, rows)) {
    q("DELETE FROM processes WHERE id=?").run(processId);
    throw new Error("device connection changed");
  }
  logEvent("process.created", workspaceForDevice(deviceId), userId, deviceId, { processId, command, cwd });
  return { processId };
}

export function inputProcess(userId: string, input: any) {
  const processId = String(input.processId || ""), allowed = processAccess(userId, processId);
  if (!['starting','running'].includes(allowed.process.status)) throw new Error("process is not running");
  if (!isOnline(allowed.process.device_id)) throw new Error("device is offline");
  const data = String(input.data || "");
  if (!data || data.length > 64 * 1024) throw new Error("input must be 1-65536 characters");
  if (!sendProcessControl(allowed.process.device_id, { type: "process.input", id: processId, input: data })) throw new Error("device connection changed");
  return { ok: true };
}

export function resizeRemoteProcess(userId: string, input: any) {
  const processId = String(input.processId || ""), allowed = processAccess(userId, processId);
  if (!['starting','running'].includes(allowed.process.status)) return { ok: true };
  const cols = boundedSize(input.cols, Number(allowed.process.cols || 80));
  const rows = boundedSize(input.rows, Number(allowed.process.rows || 24));
  if (!sendProcessControl(allowed.process.device_id, { type: "process.resize", id: processId, cols, rows })) throw new Error("device connection changed");
  resizeProcess(processId, cols, rows);
  return { ok: true };
}

export function signalProcess(userId: string, input: any) {
  const processId = String(input.processId || ""), allowed = processAccess(userId, processId);
  if (!['starting','running'].includes(allowed.process.status)) return { ok: true };
  const signal = String(input.signal || "TERM").toUpperCase();
  if (!["INT", "TERM", "KILL"].includes(signal)) throw new Error("unsupported signal");
  if (!sendProcessControl(allowed.process.device_id, { type: "process.signal", id: processId, signal })) throw new Error("device connection changed");
  return { ok: true };
}

export async function handleProcesses(req: Request, path: string, user: { id: string }): Promise<Response | null> {
  let match = path.match(/^\/api\/v1\/devices\/([^/]+)\/processes$/);
  if (match && req.method === "GET") {
    if (!deviceRole(user.id, match[1])) return fail("forbidden", 403);
    return json({ processes: q<any>("SELECT * FROM processes WHERE device_id=? ORDER BY created_at DESC LIMIT 100").all(match[1]).map(processJSON) });
  }
  match = path.match(/^\/api\/v1\/processes\/([^/]+)$/);
  if (match && req.method === "GET") {
    try { return json({ process: processJSON(processAccess(user.id, match[1]).process) }); }
    catch { return fail("process not found", 404); }
  }
  return null;
}
