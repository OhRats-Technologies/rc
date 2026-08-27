import { canOperate, deviceRole, logEvent } from "../core";
import { id, now, q } from "../db";
import { markProcessLost, workspaceForDevice } from "../process-store";
import type { AgentClientMessage } from "../protocol";
import { sendMcpAgent } from "./relay";
import type { McpToolContext } from "./types";

const OUTPUT_LIMIT = 256 * 1024;
const COMPLETED_TTL = 5 * 60_000;
const ACTIVE_TTL = 30 * 60_000;
const MAX_STATES = 128;

export type McpProcessResult = {
  processId: string;
  status: "exited" | "running" | "lost";
  output: string;
  exitCode: number | null;
  signal: string | null;
  error: string | null;
  nextOffset: number;
  outputTruncated: boolean;
};

type Listener = () => void;
type State = {
  processId: string; grantId: string; userId: string; deviceId: string; status: McpProcessResult["status"];
  output: string; outputTruncated: boolean; exitCode: number | null; signal: string | null; error: string | null;
  createdAt: number; updatedAt: number; initial?: { resolve: (value: McpProcessResult) => void; timer: ReturnType<typeof setTimeout> };
  listeners: Set<Listener>;
};

const states = new Map<string, State>();

function appendOutput(state: State, chunk: string) {
  if (!chunk || state.output.length >= OUTPUT_LIMIT) {
    if (chunk) state.outputTruncated = true;
    return;
  }
  const room = OUTPUT_LIMIT - state.output.length;
  state.output += chunk.slice(0, room);
  if (chunk.length > room) state.outputTruncated = true;
}

function decodeProcessData(value: string) {
  try { return Buffer.from(value, "base64url").toString("utf8"); }
  catch { return ""; }
}

function result(state: State, offset = 0): McpProcessResult {
  const start = Math.min(Math.max(0, offset), state.output.length);
  return { processId: state.processId, status: state.status, output: state.output.slice(start), exitCode: state.exitCode,
    signal: state.signal, error: state.error, nextOffset: state.output.length, outputTruncated: state.outputTruncated };
}

function notify(state: State) {
  state.updatedAt = now();
  for (const listener of [...state.listeners]) listener();
}

function cleanupStates() {
  const t = now();
  for (const [processId, state] of states) {
    const ttl = state.status === "running" ? ACTIVE_TTL : COMPLETED_TTL;
    if (state.updatedAt + ttl < t) states.delete(processId);
  }
  if (states.size < MAX_STATES) return;
  const removable = [...states.values()].sort((a, b) => a.updatedAt - b.updatedAt);
  while (states.size >= MAX_STATES && removable.length) states.delete(removable.shift()!.processId);
}

function settleInitial(state: State) {
  if (!state.initial) return;
  const initial = state.initial; delete state.initial; clearTimeout(initial.timer); initial.resolve(result(state));
}

export function runMcpProcess(context: McpToolContext, input: {
  deviceId: string; command: string; cwd?: string; timeoutSeconds?: number;
}) {
  cleanupStates();
  const { grant, payload } = context, deviceId = String(input.deviceId || "");
  if (!payload.deviceIds.includes(deviceId)) throw new Error("device is outside this MCP grant");
  if (!canOperate(deviceRole(payload.userId, deviceId))) throw new Error("operator access is no longer available for this device");
  const command = String(input.command || "").trim(), cwd = String(input.cwd || "").trim().slice(0, 4096);
  if (!command || command.length > 8192) throw new Error("invalid command");
  const processId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at)
    VALUES(?,?,'mcp','starting',0,?,?)`).run(processId, deviceId, payload.userId, t);
  logEvent("mcp.process.created", workspaceForDevice(deviceId), payload.userId, deviceId,
    { grantId: payload.id, client: payload.clientName, processId });
  const state: State = { processId, grantId: payload.id, userId: payload.userId, deviceId, status: "running", output: "",
    outputTruncated: false, exitCode: null, signal: null, error: null, createdAt: t, updatedAt: t, listeners: new Set() };
  states.set(processId, state);
  const timeout = Math.min(60, Math.max(1, Number(input.timeoutSeconds || 20))) * 1000;
  return new Promise<McpProcessResult>((resolve) => {
    state.initial = { resolve, timer: setTimeout(() => { delete state.initial; resolve(result(state)); }, timeout) };
    const sent = sendMcpAgent(deviceId, {
      type: "mcp.process.start", id: processId, command, cwd, userId: payload.userId,
      mcpGrant: grant.grant, mcpSignature: grant.grant_signature,
      grant: grant.control_grant, credentialId: grant.credential_id, assertion: grant.control_assertion,
    });
    if (!sent) {
      markMcpProcessLost(processId, "RC Node is offline"); markProcessLost(processId, "RC Node is offline");
    }
  });
}

export function markMcpProcessLost(processId: string, error: string) {
  const state = states.get(processId); if (!state || state.status !== "running") return;
  state.status = "lost"; state.error = String(error || "process lost").slice(0, 1024); settleInitial(state); notify(state);
}

export async function mcpProcessStatus(context: McpToolContext, processId: string, offset = 0, waitSeconds = 0) {
  cleanupStates();
  const state = states.get(processId);
  if (!state || state.grantId !== context.payload.id || state.userId !== context.payload.userId || !context.payload.deviceIds.includes(state.deviceId)) {
    throw new Error("process status is unavailable for this MCP grant");
  }
  const startOffset = Math.min(Math.max(0, Number(offset) || 0), state.output.length);
  const wait = Math.min(60, Math.max(0, Number(waitSeconds) || 0)) * 1000;
  if (!wait || state.status !== "running" || state.output.length > startOffset) return result(state, startOffset);
  return await new Promise<McpProcessResult>((resolve) => {
    const done = () => { clearTimeout(timer); state.listeners.delete(done); resolve(result(state, startOffset)); };
    const timer = setTimeout(done, wait);
    state.listeners.add(done);
  });
}

export function handleMcpProcessMessage(process: any, message: AgentClientMessage) {
  if (process?.origin !== "mcp") return false;
  const state = states.get(process.id);
  if (message.type === "process.stdout" || message.type === "process.stderr") {
    if (state) { appendOutput(state, decodeProcessData(message.data || "")); notify(state); }
    return true;
  }
  if (message.type === "process.exit") {
    if (state) {
      appendOutput(state, message.output || ""); state.status = "exited"; state.exitCode = message.exitCode ?? null;
      state.signal = message.signal || null; state.error = null; settleInitial(state); notify(state);
    }
    return false;
  }
  return message.type === "process.started";
}
