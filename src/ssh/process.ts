import { id, now, q } from "../db";
import { logEvent } from "../core";
import { markProcessLost, workspaceForDevice } from "../process-store";
import type { AgentClientMessage } from "../protocol";
import { sendSshAgent } from "./relay";

export type SshPrincipal = {
  userId: string; keyId: string; deviceId: string;
  proof: { grant: string; credentialId: string; assertion: string };
};

type Terminal = { cols: number; rows: number; term?: string };
type State = {
  id: string; sessionId: string; deviceId: string; userId: string; exited: boolean;
  stdout(data: Buffer): void; stderr(data: Buffer): void; exit(code: number, signal: string, error?: string): void;
};

const states = new Map<string, State>();

export function startSshProcess(principal: SshPrincipal, command: string, terminal: Terminal | undefined, handlers: {
  stdout(data: Buffer): void; stderr(data: Buffer): void; exit(code: number, signal: string, error?: string): void;
}) {
  const processId = id(), sessionId = id(), t = now();
  q(`INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at)
    VALUES(?,?,'ssh','starting',?,?,?)`).run(processId, principal.deviceId, terminal ? 1 : 0, principal.userId, t);
  const state: State = { id: processId, sessionId, deviceId: principal.deviceId, userId: principal.userId, exited: false, ...handlers };
  states.set(processId, state);
  logEvent("ssh.process.created", workspaceForDevice(principal.deviceId), principal.userId, principal.deviceId, { processId, keyId: principal.keyId });
  const sent = sendSshAgent(principal.deviceId, {
    type: "ssh.process.start", id: processId, sessionId, userId: principal.userId, command,
    ...(terminal ? { terminal } : {}), ...principal.proof,
  });
  if (!sent) {
    states.delete(processId); markProcessLost(processId, "RC Node is offline"); throw new Error("RC Node is offline");
  }
  return {
    id: processId,
    stdin(data: Buffer) {
      if (data.length) sendSshAgent(principal.deviceId, { type: "ssh.process.stdin", id: processId, sessionId, data: data.toString("base64url") });
    },
    closeStdin() { sendSshAgent(principal.deviceId, { type: "ssh.process.stdin.close", id: processId, sessionId }); },
    resize(cols: number, rows: number) { sendSshAgent(principal.deviceId, { type: "ssh.process.resize", id: processId, sessionId, cols, rows }); },
    signal(signal: string) { sendSshAgent(principal.deviceId, { type: "ssh.process.signal", id: processId, sessionId, signal }); },
    kill() { if (!state.exited) sendSshAgent(principal.deviceId, { type: "ssh.process.signal", id: processId, sessionId, signal: "KILL" }); },
  };
}

export function handleSshProcessMessage(process: any, message: AgentClientMessage) {
  const state = states.get(process?.id || "");
  if (!state) return false;
  if (message.type === "process.stdout" || message.type === "process.stderr") {
    const data = Buffer.from(message.data || "", "base64url");
    if (message.type === "process.stdout") state.stdout(data); else state.stderr(data);
    return true;
  }
  if (message.type === "process.exit") {
    state.exited = true; states.delete(state.id);
    state.exit(message.exitCode ?? -1, message.signal || "", message.output || undefined);
    return false;
  }
  return message.type === "process.started";
}
