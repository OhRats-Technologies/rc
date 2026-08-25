import { authoritySnapshot, canonicalAuthority } from "./authority";
import { controlProof, verifyClientSignature } from "./control-auth";
import { canOperate, deviceRole } from "./core";
import { q } from "./db";
import type { AgentClientMessage, AgentServerMessage, BrowserServerMessage } from "./protocol";

type RelaySocket = { send(data: string): unknown };
type Sender = (deviceId: string, message: AgentServerMessage) => boolean;
let sendAgent: Sender = () => false;
const pending = new Map<string, { socket: RelaySocket; deviceId: string; kind: "challenge" | "open" }>();
const sessions = new Map<string, { socket: RelaySocket; deviceId: string }>();

export function registerAgentSender(sender: Sender) { sendAgent = sender; }
function browserSend(socket: RelaySocket, value: BrowserServerMessage) { try { socket.send(JSON.stringify(value)); } catch {} }
function failure(socket: RelaySocket, requestId: string, error: string) {
  browserSend(socket, { type: "response", requestId, ok: false, error });
}

export function requestControlChallenge(userId: string, deviceId: string, requestId: string, socket: RelaySocket) {
  if (!canOperate(deviceRole(userId, deviceId))) throw new Error("operator required");
  pending.set(requestId, { socket, deviceId, kind: "challenge" });
  if (!sendAgent(deviceId, { type: "control.challenge", requestId })) {
    pending.delete(requestId); throw new Error("device is offline");
  }
}

export function requestControlOpen(userId: string, input: any, socket: RelaySocket, apiKeyId: string | null = null) {
  const deviceId = String(input.deviceId || ""), requestId = String(input.requestId || ""), clientId = String(input.clientId || "");
  if (!canOperate(deviceRole(userId, deviceId))) throw new Error("operator required");
  const proof = apiKeyId ? null : controlProof(userId, clientId);
  if (apiKeyId && apiKeyId !== clientId) throw new Error("API control key mismatch");
  if (!apiKeyId && !proof) throw new Error("control client authorization expired");
  pending.set(requestId, { socket, deviceId, kind: "open" });
  const sent = sendAgent(deviceId, { type: "control.open", requestId, challenge: String(input.challenge || ""), clientId,
    grant: proof?.grant || "", credentialId: proof?.credentialId || "", assertion: proof?.assertion || "",
    publicKey: String(input.publicKey || ""), signature: String(input.signature || "") });
  if (!sent) { pending.delete(requestId); throw new Error("device is offline"); }
}

export function relayControlFrame(userId: string, input: any, socket: RelaySocket) {
  const sessionId = String(input.sessionId || ""), session = sessions.get(sessionId);
  if (!session || session.socket !== socket || session.deviceId !== input.deviceId || !canOperate(deviceRole(userId, session.deviceId))) return false;
  return sendAgent(session.deviceId, { type: "control.frame", sessionId, sequence: Number(input.sequence), ciphertext: String(input.ciphertext || "") });
}

export function closeControlSession(input: any, socket: RelaySocket) {
  const sessionId = String(input.sessionId || ""), session = sessions.get(sessionId);
  if (!session || session.socket !== socket) return;
  sessions.delete(sessionId); sendAgent(session.deviceId, { type: "control.close", sessionId });
}

export function releaseControlSocket(socket: RelaySocket) {
  for (const [requestId, request] of pending) if (request.socket === socket) pending.delete(requestId);
  for (const [sessionId, session] of sessions) if (session.socket === socket) {
    sessions.delete(sessionId); sendAgent(session.deviceId, { type: "control.close", sessionId });
  }
}

export async function syncWorkspaceAuthority(userId: string, workspaceId: string, clientId: string, signature: string) {
  const proof = controlProof(userId, clientId); if (!proof) throw new Error("control client authorization expired");
  const snapshot = canonicalAuthority(authoritySnapshot(workspaceId));
  const digest = new Bun.CryptoHasher("sha256").update(snapshot).digest("hex");
  if (!await verifyClientSignature(userId, clientId, `rc-authority-v1\n${digest}`, signature)) throw new Error("invalid authority signature");
  const devices = q<{ id: string }>("SELECT id FROM devices WHERE workspace_id=?").all(workspaceId);
  let sent = 0;
  for (const device of devices) if (sendAgent(device.id, { type: "lock.sync", snapshot, grant: proof.grant,
    credentialId: proof.credentialId, assertion: proof.assertion, signature })) sent++;
  return { ok: true, devices: devices.length, online: sent, lockHash: digest };
}

export function bootstrapAuthority(deviceId: string, lockHash: string) {
  if (lockHash) return;
  const workspaceId = q<{ workspace_id: string }>("SELECT workspace_id FROM devices WHERE id=?").get(deviceId)?.workspace_id;
  if (!workspaceId) return;
  sendAgent(deviceId, { type: "lock.bootstrap", snapshot: canonicalAuthority(authoritySnapshot(workspaceId)) });
}

export function handleControlAgentMessage(deviceId: string, message: AgentClientMessage) {
  if (message.type === "control.challenge") {
    const request = pending.get(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "challenge") return true;
    pending.delete(message.requestId); browserSend(request.socket, { type: "response", requestId: message.requestId, ok: true, result: { challenge: message.challenge } }); return true;
  }
  if (message.type === "control.ready") {
    const request = pending.get(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "open") return true;
    pending.delete(message.requestId); sessions.set(message.sessionId, { socket: request.socket, deviceId });
    browserSend(request.socket, { type: "response", requestId: message.requestId, ok: true,
      result: { sessionId: message.sessionId, transportPublicKey: message.transportPublicKey,
        ephemeralPublicKey: message.ephemeralPublicKey, signature: message.signature } }); return true;
  }
  if (message.type === "control.error") {
    const requestId = message.requestId || "", request = pending.get(requestId); if (!request) return true;
    pending.delete(requestId); failure(request.socket, requestId, message.output || "control request rejected"); return true;
  }
  if (message.type === "control.frame") {
    const session = sessions.get(message.sessionId); if (session?.deviceId === deviceId) browserSend(session.socket,
      { type: "control.frame", sessionId: message.sessionId, sequence: message.sequence, ciphertext: message.ciphertext }); return true;
  }
  if (message.type === "lock.state") {
    q("UPDATE devices SET lock_hash=? WHERE id=?").run(String(message.lockHash || "").slice(0, 64), deviceId); return true;
  }
  return false;
}

