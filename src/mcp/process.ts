import { canOperate, deviceRole, logEvent } from "../core";
import { id, now, q } from "../db";
import { markProcessLost, workspaceForDevice } from "../process-store";
import type { AgentClientMessage } from "../protocol";
import { sendMcpAgent } from "./relay";
import type { McpToolContext } from "./types";

const OUTPUT_LIMIT = 256 * 1024;
type Result = { processId: string; status: "exited" | "running" | "lost"; output: string; exitCode: number | null; signal: string | null };
type Waiter = { output: string; resolve: (value: Result) => void; timer: ReturnType<typeof setTimeout> };
const waiters = new Map<string, Waiter>();

function safeOutput(value: string, chunk: string) {
  if (value.length >= OUTPUT_LIMIT) return value;
  return (value + chunk).slice(0, OUTPUT_LIMIT);
}

export function runMcpProcess(context: McpToolContext, input: {
  deviceId: string; command: string; cwd?: string; kind: "terminal" | "action"; actionId?: string; actionHash?: string; timeoutSeconds?: number;
}) {
  const { grant, payload } = context, deviceId = String(input.deviceId || "");
  if (!payload.deviceIds.includes(deviceId)) throw new Error("device is outside this MCP grant");
  if (!canOperate(deviceRole(payload.userId, deviceId))) throw new Error("operator access is no longer available for this device");
  const command = String(input.command || "").trim(), cwd = String(input.cwd || "").trim().slice(0, 4096);
  if (!command || command.length > 8192) throw new Error("invalid command");
  const processId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,command,cwd,status,encrypted,mcp,cols,rows,created_by,created_at)
    VALUES(?,?,?,NULL,'starting',1,1,80,24,?,?)`).run(processId, deviceId, "[mcp]", payload.userId, t);
  logEvent("mcp.process.created", workspaceForDevice(deviceId), payload.userId, deviceId,
    { grantId: payload.id, client: payload.clientName, processId, kind: input.kind, actionId: input.actionId || null });
  const timeout = Math.min(60, Math.max(1, Number(input.timeoutSeconds || 20))) * 1000;
  return new Promise<Result>((resolve) => {
    const timer = setTimeout(() => {
      const waiter = waiters.get(processId); waiters.delete(processId);
      resolve({ processId, status: "running", output: waiter?.output || "", exitCode: null, signal: null });
    }, timeout);
    waiters.set(processId, { output: "", resolve, timer });
    const sent = sendMcpAgent(deviceId, {
      type: "mcp.process.start", id: processId, command, cwd, userId: payload.userId, mcpKind: input.kind,
      actionId: input.actionId || "", mcpGrant: grant.grant, mcpSignature: grant.grant_signature,
      grant: grant.control_grant, credentialId: grant.credential_id, assertion: grant.control_assertion,
    });
    if (!sent) {
      clearTimeout(timer); waiters.delete(processId); markProcessLost(processId, "RC Node is offline");
      resolve({ processId, status: "lost", output: "", exitCode: null, signal: null });
    }
  });
}

export function handleMcpProcessMessage(process: any, message: AgentClientMessage) {
  if (!process?.mcp) return false;
  const waiter = waiters.get(process.id);
  if (message.type === "process.output") {
    if (waiter) waiter.output = safeOutput(waiter.output, message.output || "");
    return true;
  }
  if (message.type === "process.exit") {
    if (waiter) {
      if (message.output) waiter.output = safeOutput(waiter.output, message.output);
      clearTimeout(waiter.timer); waiters.delete(process.id);
      waiter.resolve({ processId: process.id, status: "exited", output: waiter.output,
        exitCode: message.exitCode ?? null, signal: message.signal || null });
    }
    return false;
  }
  return message.type === "process.started";
}
