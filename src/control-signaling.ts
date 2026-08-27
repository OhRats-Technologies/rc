import { authoritySnapshot, canonicalAuthority } from "./authority";
import { controlProof, verifyClientSignature } from "./control-auth";
import { canOperate, deviceRole } from "./core";
import { q } from "./db";
import type { AgentClientMessage, AgentServerMessage, BrowserServerMessage } from "./protocol";
import { controlIceServers, type IceServer } from "./webrtc";

type ControlSocket = { send(data: string): unknown };
type Sender = (deviceId: string, message: AgentServerMessage) => boolean;
let sendAgent: Sender = () => false;
type PendingRequest =
  | { socket: ControlSocket; deviceId: string; kind: "challenge" | "webrtc" }
  | { socket: ControlSocket; deviceId: string; kind: "open"; iceServers: IceServer[] };
const pending = new Map<string, PendingRequest>();
const sessions = new Map<string, { socket: ControlSocket; deviceId: string; iceServers: IceServer[] }>();
const transportDiagnostics = new Map<string, { deviceId: string; userId: string; detail: Record<string, unknown>; at: number }>();
const MAX_TRANSPORT_DIAGNOSTICS = 256;

export function recentControlTransport(deviceId: string) {
  return [...transportDiagnostics.values()].filter(item => item.deviceId === deviceId).sort((a, b) => b.at - a.at)[0] || null;
}

export function registerAgentSender(sender: Sender) { sendAgent = sender; }
function browserSend(socket: ControlSocket, value: BrowserServerMessage) { try { socket.send(JSON.stringify(value)); } catch {} }
function failure(socket: ControlSocket, requestId: string, error: string) {
  browserSend(socket, { type: "response", requestId, ok: false, error });
}

export function requestControlChallenge(userId: string, deviceId: string, requestId: string, socket: ControlSocket) {
  if (!canOperate(deviceRole(userId, deviceId))) throw new Error("operator required");
  pending.set(requestId, { socket, deviceId, kind: "challenge" });
  if (!sendAgent(deviceId, { type: "control.challenge", requestId })) {
    pending.delete(requestId); throw new Error("device is offline");
  }
}

export async function requestControlOpen(userId: string, input: any, socket: ControlSocket, apiKeyId: string | null = null) {
  const deviceId = String(input.deviceId || ""), requestId = String(input.requestId || ""), clientId = String(input.clientId || "");
  if (!canOperate(deviceRole(userId, deviceId))) throw new Error("operator required");
  const proof = apiKeyId ? null : controlProof(userId, clientId);
  if (apiKeyId && apiKeyId !== clientId) throw new Error("API control key mismatch");
  if (!apiKeyId && !proof) throw new Error("control client authorization expired");
  const iceServers = await controlIceServers();
  pending.set(requestId, { socket, deviceId, kind: "open", iceServers });
  const sent = sendAgent(deviceId, { type: "control.open", requestId, challenge: String(input.challenge || ""), clientId,
    grant: proof?.grant || "", credentialId: proof?.credentialId || "", assertion: proof?.assertion || "",
    publicKey: String(input.publicKey || ""), signature: String(input.signature || "") });
  if (!sent) { pending.delete(requestId); throw new Error("device is offline"); }
}

export function requestControlWebRTC(userId: string, input: any, socket: ControlSocket) {
  const sessionId = String(input.sessionId || ""), requestId = String(input.requestId || ""), session = sessions.get(sessionId);
  if (!session || session.socket !== socket || session.deviceId !== input.deviceId || !canOperate(deviceRole(userId, session.deviceId))) {
    throw new Error("control session unavailable");
  }
  pending.set(requestId, { socket, deviceId: session.deviceId, kind: "webrtc" });
  if (!sendAgent(session.deviceId, { type: "control.webrtc", requestId, sessionId, sdp: String(input.sdp || ""), iceServers: session.iceServers })) {
    pending.delete(requestId); throw new Error("device is offline");
  }
}

export function reportControlTransport(userId: string, input: any, socket: ControlSocket) {
  const sessionId = String(input.sessionId || ""), session = sessions.get(sessionId);
  if (!session || session.socket !== socket || session.deviceId !== input.deviceId || !canOperate(deviceRole(userId, session.deviceId))) return;
  const detail = {
    transport: input.transport,
    reason: input.reason || null,
    iceState: input.iceState || null,
    connectionState: input.connectionState || null,
    localCandidates: input.localCandidates || null,
    remoteCandidates: input.remoteCandidates || null,
    selected: input.selected || null,
  };
  transportDiagnostics.set(sessionId, { deviceId: session.deviceId, userId, detail, at: Date.now() });
  while (transportDiagnostics.size > MAX_TRANSPORT_DIAGNOSTICS) transportDiagnostics.delete(transportDiagnostics.keys().next().value!);
}

export function closeControlSession(input: any, socket: ControlSocket) {
  const sessionId = String(input.sessionId || ""), session = sessions.get(sessionId);
  if (!session || session.socket !== socket) return;
  sessions.delete(sessionId); sendAgent(session.deviceId, { type: "control.close", sessionId });
}

export function releaseControlSocket(socket: ControlSocket) {
  for (const [requestId, request] of pending) if (request.socket === socket) pending.delete(requestId);
  for (const [sessionId, session] of sessions) if (session.socket === socket) {
    sessions.delete(sessionId); sendAgent(session.deviceId, { type: "control.close", sessionId });
  }
}

export async function syncWorkspaceAuthority(userId: string, workspaceId: string, clientId: string,
  transitions: Array<{ fromHash: string; generation: number; signature: string }>) {
  const proof = controlProof(userId, clientId); if (!proof) throw new Error("control client authorization expired");
  const snapshot = canonicalAuthority(authoritySnapshot(workspaceId));
  const digest = new Bun.CryptoHasher("sha256").update(snapshot).digest("hex");
  const signatures = new Map<string, string>();
  for (const transition of transitions) {
    const fromHash = String(transition.fromHash || "").toLowerCase(), generation = Number(transition.generation), signature = String(transition.signature || "");
    const key = `${generation}:${fromHash}`;
    if (!/^[0-9a-f]{64}$/.test(fromHash) || !Number.isSafeInteger(generation) || generation < 0 || signatures.has(key)) throw new Error("invalid authority transition");
    if (!await verifyClientSignature(userId, clientId, `rc-authority-v3\n${generation}\n${fromHash}\n${digest}`, signature)) {
      throw new Error("invalid authority transition signature");
    }
    signatures.set(key, signature);
  }
  const devices = q<{ id: string; lock_hash: string; lock_generation: number }>("SELECT id,lock_hash,lock_generation FROM devices WHERE workspace_id=?").all(workspaceId);
  let sent = 0;
  for (const device of devices) {
    const previousHash = String(device.lock_hash || "").toLowerCase(), previousGeneration = Number(device.lock_generation || 0);
    const signature = signatures.get(`${previousGeneration}:${previousHash}`);
    if (!signature) continue;
    if (sendAgent(device.id, { type: "lock.sync", snapshot, previousHash, previousGeneration, grant: proof.grant,
      credentialId: proof.credentialId, assertion: proof.assertion, signature })) sent++;
  }
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
    pending.delete(message.requestId); sessions.set(message.sessionId, { socket: request.socket, deviceId, iceServers: request.iceServers });
    browserSend(request.socket, { type: "response", requestId: message.requestId, ok: true,
      result: { sessionId: message.sessionId, transportPublicKey: message.transportPublicKey,
        ephemeralPublicKey: message.ephemeralPublicKey, signature: message.signature, iceServers: request.iceServers } }); return true;
  }
  if (message.type === "control.webrtc.ready") {
    const request = pending.get(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "webrtc") return true;
    pending.delete(message.requestId); browserSend(request.socket, { type: "response", requestId: message.requestId, ok: true,
      result: { sdp: message.sdp } }); return true;
  }
  if (message.type === "control.error") {
    const requestId = message.requestId || "", request = pending.get(requestId); if (!request) return true;
    pending.delete(requestId); failure(request.socket, requestId, message.output || "control request rejected"); return true;
  }
  if (message.type === "lock.state") {
    q("UPDATE devices SET lock_hash=?,lock_generation=? WHERE id=?")
      .run(String(message.lockHash || "").slice(0, 64), Number(message.lockGeneration || 0), deviceId); return true;
  }
  return false;
}

