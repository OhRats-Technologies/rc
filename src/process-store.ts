import { now, q } from "./db";

export type ProcessOrigin = "browser" | "cli" | "api" | "mcp" | "ssh" | "control" | "legacy";

export function workspaceForDevice(deviceId: string) {
  return q<any>("SELECT workspace_id FROM devices WHERE id=?").get(deviceId)?.workspace_id || null;
}

export function processRow(processId: string) {
  return q<any>("SELECT * FROM processes WHERE id=?").get(processId) || null;
}

export function isDirectControlProcess(row: any) {
  return ["browser", "cli", "api", "control"].includes(String(row?.origin || ""));
}

export function processJSON(row: any) {
  return {
    id: row.id, device_id: row.device_id, origin: String(row.origin || "legacy") as ProcessOrigin,
    status: row.status, terminal: Boolean(row.terminal), exit_code: row.exit_code, signal: row.signal,
    error: row.error, created_by: row.created_by, created_by_name: row.created_by_name || null,
    created_at: row.created_at, started_at: row.started_at, completed_at: row.completed_at,
  };
}

export function markProcessStarted(processId: string) {
  q("UPDATE processes SET status='running',started_at=coalesce(started_at,?) WHERE id=? AND status='starting'").run(now(), processId);
}

export function markProcessExited(processId: string, exitCode: number | null, signal: string | null) {
  q("UPDATE processes SET status='exited',exit_code=?,signal=?,completed_at=? WHERE id=? AND status IN ('starting','running')")
    .run(exitCode, signal, now(), processId);
}

export function markProcessLost(processId: string, error: string) {
  q("UPDATE processes SET status='lost',error=?,completed_at=? WHERE id=? AND status IN ('starting','running')").run(error, now(), processId);
}
