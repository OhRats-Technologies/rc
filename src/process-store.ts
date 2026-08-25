import { now, q } from "./db";

const OUTPUT_LIMIT = 1_000_000;
const OUTPUT_HALF = OUTPUT_LIMIT / 2;

export function workspaceForDevice(deviceId: string) {
  return q<any>("SELECT workspace_id FROM devices WHERE id=?").get(deviceId)?.workspace_id || null;
}

export function processRow(processId: string) {
  return q<any>("SELECT * FROM processes WHERE id=?").get(processId) || null;
}

export function processOutput(row: any) {
  const head = String(row.output_head || ""), tail = String(row.output_tail || "");
  const omitted = Math.max(0, Number(row.output_chars || 0) - head.length - tail.length);
  return omitted > 0 ? `${head}\n... output truncated (${omitted} characters omitted) ...\n${tail}` : head + tail;
}

export function processJSON(row: any) {
  const head = String(row.output_head || ""), tail = String(row.output_tail || "");
  return {
    id: row.id, device_id: row.device_id, command: row.command, cwd: row.cwd, status: row.status,
    encrypted: Boolean(row.encrypted),
    output: processOutput(row), output_truncated: Number(row.output_chars || 0) > head.length + tail.length,
    revision: row.revision, cols: row.cols, rows: row.rows, exit_code: row.exit_code, signal: row.signal,
    error: row.error, created_by: row.created_by, created_by_name: row.created_by_name || null,
    created_at: row.created_at, started_at: row.started_at, completed_at: row.completed_at,
  };
}

export function appendProcessOutput(processId: string, chunk: string) {
  const row = processRow(processId);
  if (!row || !chunk) return row ? Number(row.revision || 0) : null;
  let head = String(row.output_head || ""), tail = String(row.output_tail || "");
  const previousTotal = Number(row.output_chars || 0), total = previousTotal + chunk.length;
  if (total <= OUTPUT_LIMIT) head += chunk;
  else if (previousTotal <= OUTPUT_LIMIT) {
    const full = head + tail + chunk;
    head = full.slice(0, OUTPUT_HALF); tail = full.slice(-OUTPUT_HALF);
  } else tail = (tail + chunk).slice(-OUTPUT_HALF);
  const revision = Number(row.revision || 0) + 1;
  q("UPDATE processes SET output_head=?,output_tail=?,output_chars=?,revision=? WHERE id=?")
    .run(head, tail, total, revision, processId);
  return revision;
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

export function resizeProcess(processId: string, cols: number, rows: number) {
  q("UPDATE processes SET cols=?,rows=? WHERE id=?").run(cols, rows, processId);
}
