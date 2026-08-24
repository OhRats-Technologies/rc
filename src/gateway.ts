import { createPublicKey, verify as verifySignature } from "node:crypto";
import { logEvent } from "./core";
import { now, q } from "./db";
import { publishEvent } from "./events";

export type AgentData = { kind: "agent"; deviceId: string };
const agents = new Map<string, ServerWebSocket<AgentData>>();
const agentActivity = new Map<string, number>();

export function agentsCount() { return agents.size; }
export function isOnline(deviceId: string) { return agents.has(deviceId); }
export function disconnectDevice(deviceId: string) {
  agents.get(deviceId)?.close(1008, "device removed");
  agents.delete(deviceId);
  agentActivity.delete(deviceId);
}

export function dispatchJob(jobId: string, deviceId: string, command: string) {
  const ws = agents.get(deviceId);
  if (!ws) return false;
  const claimed = q("UPDATE jobs SET status='sent' WHERE id=? AND status='pending'").run(jobId);
  if (!claimed.changes) return false;
  try {
    ws.send(JSON.stringify({ type: "job", id: jobId, command }));
    return true;
  } catch {
    q("UPDATE jobs SET status='pending' WHERE id=? AND status='sent' AND started_at IS NULL").run(jobId);
    return false;
  }
}

function sendPending(deviceId: string) {
  const rows = q<any>("SELECT id,payload FROM jobs WHERE device_id=? AND status='pending' ORDER BY created_at LIMIT 20").all(deviceId);
  for (const row of rows) dispatchJob(row.id, deviceId, JSON.parse(row.payload).command);
}

export function verifyAgent(url: URL): string | null {
  const deviceId = url.searchParams.get("device") || "", ts = url.searchParams.get("ts") || "", sig = url.searchParams.get("sig") || "";
  const seconds = Number(ts);
  if (!deviceId || !Number.isFinite(seconds) || Math.abs(Date.now() / 1000 - seconds) > 60 || !sig) return null;
  const row = q<any>("SELECT public_key FROM devices WHERE id=?").get(deviceId);
  if (!row) return null;
  try {
    return verifySignature(null, Buffer.from(`relay:${deviceId}:${ts}`), createPublicKey(row.public_key), Buffer.from(sig, "base64url"))
      ? deviceId : null;
  } catch { return null; }
}

function workspaceForDevice(deviceId: string) {
  return q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id
    WHERE fd.device_id=? LIMIT 1`).get(deviceId)?.workspace_id || null;
}

export function recoverInterruptedJobs() {
  const rows = q<any>("SELECT id,device_id FROM jobs WHERE status='sent'").all();
  if (!rows.length) return;
  q("UPDATE jobs SET status='failed',result=coalesce(result,'') || ?,completed_at=? WHERE status='sent'")
    .run("\n[control plane restarted during execution]", now());
  for (const row of rows) {
    logEvent("job.interrupted", workspaceForDevice(row.device_id), null, row.device_id, { jobId: row.id, reason: "control-plane-restart" });
  }
}

setInterval(() => {
  const cutoff = Date.now() - 35_000;
  for (const [deviceId, ws] of agents) {
    if ((agentActivity.get(deviceId) || 0) >= cutoff) continue;
    ws.close(1011, "heartbeat timeout");
  }
}, 10_000).unref();

export const websocketHandlers = {
  open(ws: ServerWebSocket<AgentData>) {
    const { deviceId } = ws.data, previous = agents.get(deviceId);
    if (previous && previous !== ws) previous.close(1012, "replaced");
    agents.set(deviceId, ws);
    agentActivity.set(deviceId, Date.now());
    q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
    logEvent("device.online", workspaceForDevice(deviceId), null, deviceId);
    sendPending(deviceId);
  },
  message(ws: ServerWebSocket<AgentData>, raw: string | Buffer) {
    try {
      const msg = JSON.parse(typeof raw === "string" ? raw : Buffer.from(raw as any).toString("utf8"));
      const deviceId = ws.data.deviceId;
      agentActivity.set(deviceId, Date.now());
      if (msg.type === "hello") {
        const capabilities = Array.isArray(msg.capabilities) ? msg.capabilities.map(String).slice(0, 32) : [];
        q(`UPDATE devices SET hostname=?,platform=?,arch=?,agent_version=?,capabilities=?,last_seen=? WHERE id=?`).run(
          String(msg.hostname || "unknown").slice(0, 255), String(msg.platform || "unknown").slice(0, 40),
          String(msg.arch || "unknown").slice(0, 40), String(msg.agentVersion || "unknown").slice(0, 40),
          JSON.stringify(capabilities), now(), deviceId,
        );
        publishEvent({ kind: "device.updated", workspaceId: workspaceForDevice(deviceId), deviceId });
        return;
      }
      if (msg.type === "heartbeat") {
        q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
        return;
      }
      if (msg.type === "started" && msg.id) {
        const job = q<any>("SELECT id,session_id FROM jobs WHERE id=? AND device_id=?").get(String(msg.id), deviceId);
        if (!job) return;
        q("UPDATE jobs SET started_at=coalesce(started_at,?) WHERE id=? AND status='sent'").run(now(), job.id);
        publishEvent({ kind: "job.started", workspaceId: workspaceForDevice(deviceId), deviceId, sessionId: job.session_id, jobId: job.id });
        return;
      }
      if (msg.type === "output" && msg.id) {
        const job = q<any>("SELECT id,session_id FROM jobs WHERE id=? AND device_id=?").get(String(msg.id), deviceId);
        if (!job) return;
        const chunk = String(msg.output || "").slice(0, 64 * 1024);
        q("UPDATE jobs SET result=substr(coalesce(result,'') || ?,1,1048576) WHERE id=?").run(chunk, job.id);
        publishEvent({ kind: "job.output", workspaceId: workspaceForDevice(deviceId), deviceId, sessionId: job.session_id, jobId: job.id, detail: { chunk } });
        return;
      }
      if (msg.type === "result" && msg.id) {
        const job = q<any>("SELECT id,session_id FROM jobs WHERE id=? AND device_id=?").get(String(msg.id), deviceId);
        if (!job) return;
        const output = String(msg.output || "").slice(0, 1024 * 1024);
        const exitCode = Number.isInteger(msg.exitCode) ? msg.exitCode : -1;
        if (output) q("UPDATE jobs SET result=substr(coalesce(result,'') || ?,1,1048576) WHERE id=?").run(output, job.id);
        q("UPDATE jobs SET status=?,exit_code=?,completed_at=? WHERE id=?")
          .run(exitCode === 0 ? "completed" : "failed", exitCode, now(), job.id);
        q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
        publishEvent({ kind: "job.finished", workspaceId: workspaceForDevice(deviceId), deviceId, sessionId: job.session_id, jobId: job.id,
          detail: { status: exitCode === 0 ? "completed" : "failed", exitCode } });
      }
    } catch (error) { console.error("agent message", error); }
  },
  close(ws: ServerWebSocket<AgentData>) {
    const { deviceId } = ws.data;
    if (agents.get(deviceId) !== ws) return;
    agents.delete(deviceId);
    agentActivity.delete(deviceId);
    const running = q<any>("SELECT id,session_id,started_at FROM jobs WHERE device_id=? AND status='sent'").all(deviceId);
    for (const job of running) {
      const message = job.started_at ? "\n[device disconnected during execution]" : "\n[device disconnected before acknowledgement]";
      q("UPDATE jobs SET status='failed',result=coalesce(result,'') || ?,completed_at=? WHERE id=? AND status='sent'")
        .run(message, now(), job.id);
      publishEvent({ kind: "job.finished", workspaceId: workspaceForDevice(deviceId), deviceId, sessionId: job.session_id, jobId: job.id,
        detail: { status: "failed", message } });
    }
    logEvent("device.offline", workspaceForDevice(deviceId), null, deviceId);
  },
};
