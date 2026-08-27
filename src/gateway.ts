import { logEvent } from "./core";
import { now, opaque, q, sha } from "./db";
import { base64urlToBytes, pemPublicKeyToDer } from "./encoding";
import { publishEvent } from "./events";
import { isDirectControlProcess, markProcessExited, markProcessLost, markProcessStarted, processRow, workspaceForDevice } from "./process-store";
import type { AgentClientMessage, AgentServerMessage } from "./protocol";
import type { SocketWriter } from "./browser-socket";
import { AGENT_CHALLENGE_TTL } from "./config";
import { bootstrapAuthority, handleControlAgentMessage, registerAgentSender } from "./control-relay";
import { handleMcpProcessMessage, markMcpProcessLost } from "./mcp/process";
import { registerMcpSender } from "./mcp/relay";
import { handleSshProcessMessage } from "./ssh/process";
import { registerSshSender } from "./ssh/relay";

const agents = new Map<string, SocketWriter>();
const agentActivity = new Map<string, number>();
const disconnectTimers = new Map<string, ReturnType<typeof setTimeout>>();
const RECONNECT_GRACE_MS = 45_000;

export function agentsCount() { return agents.size; }
export function isOnline(deviceId: string) { return agents.has(deviceId); }
export function disconnectDevice(deviceId: string, remove = false) {
  const timer = disconnectTimers.get(deviceId); if (timer) clearTimeout(timer);
  disconnectTimers.delete(deviceId);
  const ws = agents.get(deviceId);
  ws?.close(1008, "device removed");
  agents.delete(deviceId);
  agentActivity.delete(deviceId);
}

function send(deviceId: string, message: AgentServerMessage) {
  const ws = agents.get(deviceId);
  if (!ws) return false;
  try { ws.send(JSON.stringify(message)); return true; } catch { return false; }
}

registerAgentSender(send);
registerMcpSender(send);
registerSshSender(send);

export function createAgentChallenge(deviceId: string) {
  const challenge = opaque("agent"), t = now();
  q("DELETE FROM agent_auth_challenges WHERE expires_at<=?").run(t);
  q("DELETE FROM revoked_agent_auth_challenges WHERE expires_at<=?").run(t);
  if (q("SELECT 1 FROM devices WHERE id=?").get(deviceId)) {
    q("INSERT INTO agent_auth_challenges(challenge_hash,device_id,created_at,expires_at) VALUES(?,?,?,?)")
      .run(sha(challenge), deviceId, t, t + AGENT_CHALLENGE_TTL);
  } else if (q("SELECT 1 FROM revoked_devices WHERE id=?").get(deviceId)) {
    q("INSERT INTO revoked_agent_auth_challenges(challenge_hash,device_id,created_at,expires_at) VALUES(?,?,?,?)")
      .run(sha(challenge), deviceId, t, t + AGENT_CHALLENGE_TTL);
  } else return null;
  return { challenge, expiresAt: t + AGENT_CHALLENGE_TTL };
}

export async function verifyAgent(request: Request, deviceId: string): Promise<string | null> {
  const challenge = request.headers.get("x-rc-challenge") || "";
  const sig = request.headers.get("x-rc-signature") || "";
  if (!deviceId || !challenge || !sig) return null;
  let revoked = false;
  let row = q<any>(`SELECT d.public_key FROM agent_auth_challenges c JOIN devices d ON d.id=c.device_id
    WHERE c.challenge_hash=? AND c.device_id=? AND c.expires_at>?`).get(sha(challenge), deviceId, now());
  if (!row) {
    revoked = true;
    row = q<any>(`SELECT d.public_key FROM revoked_agent_auth_challenges c JOIN revoked_devices d ON d.id=c.device_id
      WHERE c.challenge_hash=? AND c.device_id=? AND c.expires_at>?`).get(sha(challenge), deviceId, now());
  }
  if (!row) return null;
  try {
    const key = await crypto.subtle.importKey("spki", pemPublicKeyToDer(row.public_key), { name: "Ed25519" }, false, ["verify"]);
    const path = new URL(request.url).pathname;
    const payload = `rc-auth-v2\n${deviceId}\n${challenge}\n${request.method}\n${path}`;
    const ok = await crypto.subtle.verify("Ed25519", key, base64urlToBytes(sig), new TextEncoder().encode(payload));
    if (!ok) return null;
    const table = revoked ? "revoked_agent_auth_challenges" : "agent_auth_challenges";
    const consumed = q(`DELETE FROM ${table} WHERE challenge_hash=? AND device_id=? AND expires_at>?`)
      .run(sha(challenge), deviceId, now()).changes;
    return consumed === 1 ? deviceId : null;
  } catch { return null; }
}

function scheduleDisconnect(deviceId: string, startup = false) {
  const old = disconnectTimers.get(deviceId); if (old) clearTimeout(old);
  disconnectTimers.set(deviceId, setTimeout(() => {
    disconnectTimers.delete(deviceId);
    if (agents.has(deviceId)) return;
    const running = q<any>("SELECT id,status FROM processes WHERE device_id=? AND status IN ('starting','running')").all(deviceId);
    for (const process of running) {
      const error = startup ? "control plane restarted and device did not reconnect" : process.status === "running" ? "device disconnected beyond reconnect grace" : "device disconnected before acknowledgement";
      markMcpProcessLost(process.id, error);
      markProcessLost(process.id, error);
      publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id, detail: { error } });
    }
    logEvent(startup ? "device.reconnect.timeout" : "device.offline", workspaceForDevice(deviceId), null, deviceId, { reconnectGraceMs: RECONNECT_GRACE_MS });
  }, RECONNECT_GRACE_MS));
}

export function recoverInterruptedProcesses() {
  const devices = q<{ device_id: string }>("SELECT DISTINCT device_id FROM processes WHERE status IN ('starting','running')").all();
  for (const row of devices) scheduleDisconnect(row.device_id, true);
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
    const reconnecting = disconnectTimers.has(deviceId);
    const timer = disconnectTimers.get(deviceId); if (timer) clearTimeout(timer);
    disconnectTimers.delete(deviceId);
    const previous = agents.get(deviceId);
    if (previous && previous !== socket) previous.close(1012, "replaced");
    agents.set(deviceId, socket);
    agentActivity.set(deviceId, Date.now());
    q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
    if (reconnecting) publishEvent({ kind: "device.online", workspaceId: workspaceForDevice(deviceId), deviceId });
    else logEvent("device.online", workspaceForDevice(deviceId), null, deviceId);
  },
  message(deviceId: string, msg: AgentClientMessage) {
    try {
      agentActivity.set(deviceId, Date.now());
      if (handleControlAgentMessage(deviceId, msg)) return;
      if (msg.type === "hello") {
        const capabilities = msg.capabilities;
        q(`UPDATE devices SET hostname=?,platform=?,arch=?,agent_version=?,capabilities=?,transport_public_key=?,lock_hash=?,lock_generation=?,last_seen=? WHERE id=?`).run(
          String(msg.hostname || "unknown").slice(0, 255), String(msg.platform || "unknown").slice(0, 40),
          String(msg.arch || "unknown").slice(0, 40), String(msg.agentVersion || "unknown").slice(0, 40),
          JSON.stringify(capabilities), String(msg.transportPublicKey || "").slice(0, 100), String(msg.lockHash || "").slice(0, 64),
          Number(msg.lockGeneration || 0), Date.now(), deviceId,
        );
        bootstrapAuthority(deviceId, String(msg.lockHash || ""));
        publishEvent({ kind: "device.updated", workspaceId: workspaceForDevice(deviceId), deviceId });
        return;
      }
      if (msg.type === "heartbeat") {
        q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
        return;
      }
      if (msg.type === "process.sync") {
        const active = new Set(msg.ids);
        const hosted = q<any>("SELECT id,status FROM processes WHERE device_id=? AND status IN ('starting','running')").all(deviceId);
        for (const process of hosted) {
          if (active.has(process.id)) {
            if (process.status === "starting") {
              markProcessStarted(process.id);
              publishEvent({ kind: "process.started", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id });
            }
            continue;
          }
          const error = "RC Node reconnected without this process";
          markMcpProcessLost(process.id, error);
          markProcessLost(process.id, error);
          publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id, detail: { error } });
        }
        return;
      }
      if (msg.type === "process.start.request") {
        const process = processRow(msg.id);
        if (process && process.device_id === deviceId && isDirectControlProcess(process) && process.status === "starting" && process.created_by === msg.userId) {
          send(deviceId, { type: "process.permit", id: process.id, userId: msg.userId });
        }
        return;
      }
      if (msg.type === "process.started") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        if (handleSshProcessMessage(process, msg)) return;
        if (handleMcpProcessMessage(process, msg)) return;
        if (process.status !== "starting") return;
        markProcessStarted(process.id);
        publishEvent({ kind: "process.started", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id });
        return;
      }
      if (msg.type === "process.stdout" || msg.type === "process.stderr") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        if (handleSshProcessMessage(process, msg)) return;
        if (handleMcpProcessMessage(process, msg)) return;
        return;
      }
      if (msg.type === "process.exit") {
        const process = processRow(msg.id);
        if (!process || process.device_id !== deviceId) return;
        handleSshProcessMessage(process, msg);
        const mcp = handleMcpProcessMessage(process, msg);
        const exitCode = msg.exitCode ?? null;
        const signal = msg.signal ? String(msg.signal).slice(0, 32) : null;
        markProcessExited(process.id, exitCode, signal);
        q("UPDATE devices SET last_seen=? WHERE id=?").run(Date.now(), deviceId);
        publishEvent({ kind: "process.exited", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id,
          detail: { exitCode, signal, ...(mcp ? { mcp: true } : {}) } });
        return;
      }
      if (msg.type === "node.update.ready") {
        const running = q<any>("SELECT id FROM processes WHERE device_id=? AND status IN ('starting','running')").all(deviceId);
        for (const process of running) {
          markMcpProcessLost(process.id, "RC Node updated during execution");
          markProcessLost(process.id, "RC Node updated during execution");
          publishEvent({ kind: "process.lost", workspaceId: workspaceForDevice(deviceId), deviceId, processId: process.id,
            detail: { error: "RC Node updated during execution" } });
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
    publishEvent({ kind: "device.offline", workspaceId: workspaceForDevice(deviceId), deviceId });
    scheduleDisconnect(deviceId);
  },
};
