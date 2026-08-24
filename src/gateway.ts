import { createPublicKey, verify as verifySignature } from "node:crypto";
import { logEvent } from "./core";
import { now, q } from "./db";

export type AgentData = { kind: "agent"; deviceId: string };
const agents = new Map<string, ServerWebSocket<AgentData>>();

export function agentsCount() { return agents.size; }
export function isOnline(deviceId: string) { return agents.has(deviceId); }
export function disconnectDevice(deviceId: string) {
  agents.get(deviceId)?.close(1008, "device removed");
  agents.delete(deviceId);
}

export function dispatchJob(jobId: string, deviceId: string, command: string) {
  const ws = agents.get(deviceId);
  if (!ws) return false;
  try {
    ws.send(JSON.stringify({ type: "job", id: jobId, command }));
    q("UPDATE jobs SET status='sent',started_at=? WHERE id=? AND status='pending'").run(now(), jobId);
    return true;
  } catch { return false; }
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

export const websocketHandlers = {
  open(ws: ServerWebSocket<AgentData>) {
    const { deviceId } = ws.data, previous = agents.get(deviceId);
    if (previous && previous !== ws) previous.close(1012, "replaced");
    agents.set(deviceId, ws);
    q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
    logEvent("device.online", workspaceForDevice(deviceId), null, deviceId);
    sendPending(deviceId);
  },
  message(ws: ServerWebSocket<AgentData>, raw: string | Buffer) {
    try {
      const msg = JSON.parse(typeof raw === "string" ? raw : Buffer.from(raw as any).toString("utf8"));
      const deviceId = ws.data.deviceId;
      if (msg.type === "heartbeat") {
        q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
        return;
      }
      if (msg.type === "result" && msg.id) {
        const job = q<any>("SELECT id FROM jobs WHERE id=? AND device_id=?").get(String(msg.id), deviceId);
        if (!job) return;
        const output = String(msg.output || "").slice(0, 1024 * 1024);
        const exitCode = Number.isInteger(msg.exitCode) ? msg.exitCode : -1;
        q("UPDATE jobs SET status=?,result=?,exit_code=?,completed_at=? WHERE id=?")
          .run(exitCode === 0 ? "completed" : "failed", output, exitCode, now(), job.id);
        q("UPDATE devices SET last_seen=? WHERE id=?").run(now(), deviceId);
      }
    } catch (error) { console.error("agent message", error); }
  },
  close(ws: ServerWebSocket<AgentData>) {
    const { deviceId } = ws.data;
    if (agents.get(deviceId) !== ws) return;
    agents.delete(deviceId);
    logEvent("device.offline", workspaceForDevice(deviceId), null, deviceId);
  },
};
