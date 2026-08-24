import { logEvent } from "./core";
import { q } from "./db";
import { base64urlToBytes, pemPublicKeyToDer } from "./encoding";
import { publishEvent } from "./events";
import { appendProcessOutput, markProcessExited, markProcessLost, markProcessStarted, processRow, workspaceForDevice } from "./process-store";
import type { AgentClientMessage, AgentServerMessage } from "./protocol";
import type { SocketWriter } from "./browser-socket";

const agents = new Map<string, SocketWriter>();
const agentActivity = new Map<string, number>();

export function agentsCount() { return agents.size; }
export function isOnline(deviceId: string) { return agents.has(deviceId); }
export function disconnectDevice(deviceId: string, remove = false) {
  const ws = agents.get(deviceId);
  if (remove && ws) {
    try { ws.send(JSON.stringify({ type: "node.remove" })); } catch {}
  }
  ws?.close(1008, "device removed");
  agents.delete(deviceId);
  agentActivity.delete(deviceId);
}

function send(deviceId: string, message: AgentServerMessage) {
  const ws = agents.get(deviceId);
  if (!ws) return false;
  try { ws.send(JSON.stringify(message)); return true; } catch { return false; }
}

function capabilities(deviceId: string) {
  const raw = q<any>("SELECT capabilities FROM devices WHERE id=?").get(deviceId)?.capabilities || "[]";
  try { return JSON.parse(raw) as string[]; } catch { return []; }
}

export function dispatchProcessStart(processId: string, deviceId: string, command: string, cwd: string | null, cols: number, rows: number) {
  if (!capabilities(deviceId).includes("process")) return false;
  return send(deviceId, { type: "process.start", id: processId, command, cwd, cols, rows });
}

export function sendProcessControl(deviceId: string, message: AgentServerMessage) {
  if (!capabilities(deviceId).includes("process")) return false;
  return send(deviceId, message);
}

export function sendNodeUpdate(deviceId: string) {
  if (!capabilities(deviceId).includes("update")) return false;
  return send(deviceId, { type: "node.update" });
}

export async function verifyAgent(url: URL): Promise<string | null> {
  const deviceId = url.searchParams.get("device") || "", ts = url.searchParams.get("ts") || "", sig = url.searchParams.get("sig") || "";
  const seconds = Number(ts);
  if (!deviceId || !Number.isFinite(seconds) || Math.abs(Date.now() / 1000 - seconds) > 60 || !sig) return null;
  const row = q<any>("SELECT public_key FROM devices WHERE id=?").get(deviceId);
  if (!row) return null;
  try {
    const key = await crypto.subtle.importKey("spki", pemPublicKeyToDer(row.public_key), { name: "Ed25519" }, false, ["verify"]);
    const ok = await crypto.subtle.verify("Ed25519", key, base64urlToBytes(sig), new TextEncoder().encode(`relay:${deviceId}:${ts}`));
    return ok ? deviceId : null;
  } catch { return null; }
}

export function recoverInterruptedProcesses() {
  const rows = q<any>("SELECT id,device_id FROM processes WHERE status IN ('starting','running')").all();
  if (!rows.length) return;
  for (const row of rows) {
    markProcessLost(row.id, "control plane restarted during execution");
    logEvent("process.interrupted", workspaceForDevice(row.device_id), null, row.device_id, { processId: row.id, reason: "control-plane-restart" });
  }
}

setInterval(() => {
  const cutoff = Date.now() - 35_000;
  for (const [deviceId, ws] of agents) {
    if ((agentActivity.get(deviceId) || 0) >= cutoff) continue;
    ws.close(1011, "heartbeat timeout");
  }
}, 10_000).unref();

export const agentSocketHandlers = {
  open(deviceId: string, socket: SocketWriter) {
    const previous = agents.get(deviceId);
    if (previous && previous !== socket) previous.close(1012, "replaced");
    agents.set(deviceId, socket);
    agentActivity.set(deviceId, Date.now());
    q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
    logEvent("device.online", workspaceForDevice(deviceId), null, deviceId);
  },
  message(deviceId: string, msg: AgentClientMessage) {
    try {
      agentActivity.set(deviceId, Date.now());
      if (msg.type === "hello") {
        const capabilities = msg.capabilities;
        q(`UPDATE devices SET hostname=?,platform=?,arch=?,agent_version=?,capabilities=?,last_seen=? WHERE id=?`).run(
          String(msg.hostname || "unknown").slice(0, 255), String(msg.platform || "unknown").slice(0, 40),
          String(msg.arch || "unknown").slice(0, 40), String(msg.agentVersion || "unknown").slice(0, 40),
          JSON.stringify(capabilities), Date.now(), deviceId,
        );
        publishEvent({ kind: "device.updated", workspaceId: workspaceForDevice(deviceId), deviceId });
        return;
      }
      if (msg.type === "heartbeat") {
        q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
        return;
      }
      if (msg.type === "process.started") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        markProcessStarted(process.id);
        publishEvent({ kind: "process.started", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id });
        return;
      }
      if (msg.type === "process.output") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        const chunk = msg.output;
        const revision = appendProcessOutput(process.id, chunk);
        publishEvent({ kind: "process.output", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id, detail: { chunk, revision } });
        return;
      }
      if (msg.type === "process.exit") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        const output = msg.output || "";
        if (output) appendProcessOutput(process.id, output);
        const exitCode = msg.exitCode ?? null;
        const signal = msg.signal ? String(msg.signal).slice(0, 32) : null;
        markProcessExited(process.id, exitCode, signal);
        q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
        publishEvent({ kind: "process.exited", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id,
          detail: { exitCode, signal } });
        return;
      }
      if (msg.type === "node.update.ready") {
        const running = q<any>("SELECT id FROM processes WHERE device_id=? AND status IN ('starting','running')").all(deviceId);
        for (const process of running) {
          markProcessLost(process.id, "Relay Node updated during execution");
          publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id,
            detail: { error: "Relay Node updated during execution" } });
        }
        publishEvent({ kind: "node.update.ready", workspaceId: workspaceForDevice(deviceId), deviceId, detail: { version: msg.agentVersion || null } });
        return;
      }
      if (msg.type === "node.update.error") {
        publishEvent({ kind: "node.update.error", workspaceId: workspaceForDevice(deviceId), deviceId, detail: { error: String(msg.output || "update failed").slice(0, 1024) } });
      }
    } catch (error) { console.error("agent message", error); }
  },
  close(deviceId: string, socket: SocketWriter) {
    if (agents.get(deviceId) !== socket) return;
    agents.delete(deviceId);
    agentActivity.delete(deviceId);
    const running = q<any>("SELECT id,status FROM processes WHERE device_id=? AND status IN ('starting','running')").all(deviceId);
    for (const process of running) {
      const error = process.status === "running" ? "device disconnected during execution" : "device disconnected before acknowledgement";
      markProcessLost(process.id, error);
      publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id,
        detail: { error } });
    }
    logEvent("device.offline", workspaceForDevice(deviceId), null, deviceId);
  },
};
