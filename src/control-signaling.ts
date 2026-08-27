import { authoritySnapshot, canonicalAuthority } from "./authority";
import { controlProof, verifyClientSignature } from "./control-auth";
import { canOperate, deviceRole } from "./core";
import { q } from "./db";
import { HttpError } from "./errors";
import type { AgentClientMessage, AgentServerMessage } from "./protocol";
import { controlIceServers, type IceServer } from "./webrtc";

type Sender = (deviceId: string, message: AgentServerMessage) => boolean;
let sendAgent: Sender = () => false;

type PendingKind = "challenge" | "open" | "webrtc";
type PendingRequest = {
  kind: PendingKind;
  deviceId: string;
  userId: string;
  clientId?: string;
  iceServers?: IceServer[];
  resolve: (value: any) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};

type ControlSessionState = {
  userId: string;
  clientId: string;
  deviceId: string;
  iceServers: IceServer[];
};

const pending = new Map<string, PendingRequest>();
const sessions = new Map<string, ControlSessionState>();
const transportDiagnostics = new Map<string, { deviceId: string; userId: string; detail: Record<string, unknown>; at: number }>();
const MAX_TRANSPORT_DIAGNOSTICS = 256;
const SIGNAL_TIMEOUT_MS = 10_000;

export function recentControlTransport(deviceId: string) {
  return [...transportDiagnostics.values()].filter(item => item.deviceId === deviceId).sort((a, b) => b.at - a.at)[0] || null;
}

export function registerAgentSender(sender: Sender) { sendAgent = sender; }

function requireOperator(userId: string, deviceId: string) {
  if (!canOperate(deviceRole(userId, deviceId))) throw new HttpError(403, "operator required");
}

function requestId() { return crypto.randomUUID(); }

function waitForAgent<T>(kind: PendingKind, deviceId: string, userId: string,
  extra: Partial<Pick<PendingRequest, "clientId" | "iceServers">>, send: (id: string) => boolean): Promise<T> {
  const id = requestId();
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new HttpError(504, "RC Node signaling timed out"));
    }, SIGNAL_TIMEOUT_MS);
    timer.unref?.();
    pending.set(id, { kind, deviceId, userId, ...extra, resolve, reject, timer });
    if (!send(id)) {
      clearTimeout(timer); pending.delete(id); reject(new HttpError(409, "device is offline"));
    }
  });
}

function finishPending(id: string) {
  const request = pending.get(id);
  if (!request) return null;
  pending.delete(id); clearTimeout(request.timer); return request;
}

export function requestControlChallenge(userId: string, deviceId: string) {
  requireOperator(userId, deviceId);
  return waitForAgent<{ challenge: string }>("challenge", deviceId, userId, {}, id =>
    sendAgent(deviceId, { type: "control.challenge", requestId: id }));
}

export async function requestControlOpen(userId: string, input: {
  deviceId: string; challenge: string; clientId: string; publicKey: string; signature: string;
}, apiKeyId: string | null = null) {
  const deviceId = String(input.deviceId || ""), clientId = String(input.clientId || "");
  requireOperator(userId, deviceId);
  const proof = apiKeyId ? null : controlProof(userId, clientId);
  if (apiKeyId && apiKeyId !== clientId) throw new HttpError(403, "API control key mismatch");
  if (!apiKeyId && !proof) throw new HttpError(401, "control client authorization expired");
  const iceServers = await controlIceServers();
  return waitForAgent<{ sessionId: string; transportPublicKey: string; ephemeralPublicKey: string; signature: string; iceServers: IceServer[] }>(
    "open", deviceId, userId, { clientId, iceServers }, id => sendAgent(deviceId, {
      type: "control.open", requestId: id, challenge: String(input.challenge || ""), clientId,
      grant: proof?.grant || "", credentialId: proof?.credentialId || "", assertion: proof?.assertion || "",
      publicKey: String(input.publicKey || ""), signature: String(input.signature || ""),
    }));
}

function ownedSession(userId: string, sessionId: string, deviceId?: string) {
  const session = sessions.get(sessionId);
  if (!session || session.userId !== userId || (deviceId && session.deviceId !== deviceId)) {
    throw new HttpError(404, "control session unavailable");
  }
  requireOperator(userId, session.deviceId);
  return session;
}

export function requestControlWebRTC(userId: string, sessionId: string, input: { deviceId?: string; sdp: string }) {
  const session = ownedSession(userId, sessionId, input.deviceId);
  return waitForAgent<{ sdp: string }>("webrtc", session.deviceId, userId, {}, id =>
    sendAgent(session.deviceId, { type: "control.webrtc", requestId: id, sessionId,
      sdp: String(input.sdp || ""), iceServers: session.iceServers }));
}

export function reportControlTransport(userId: string, sessionId: string, input: any) {
  const session = ownedSession(userId, sessionId);
  const detail = {
    transport: "webrtc",
    reason: input.reason || null,
    iceState: input.iceState || null,
    connectionState: input.connectionState || null,
    localCandidates: input.localCandidates || null,
    remoteCandidates: input.remoteCandidates || null,
    selected: input.selected || null,
  };
  transportDiagnostics.set(sessionId, { deviceId: session.deviceId, userId, detail, at: Date.now() });
  while (transportDiagnostics.size > MAX_TRANSPORT_DIAGNOSTICS) transportDiagnostics.delete(transportDiagnostics.keys().next().value!);
  return { ok: true as const };
}

export function closeControlSession(userId: string, sessionId: string) {
  const session = sessions.get(sessionId);
  if (!session || session.userId !== userId) return { ok: true as const };
  sessions.delete(sessionId);
  sendAgent(session.deviceId, { type: "control.close", sessionId });
  return { ok: true as const };
}


export function releaseDeviceControlSessions(deviceId: string) {
  for (const [requestId, request] of pending) {
    if (request.deviceId !== deviceId) continue;
    pending.delete(requestId); clearTimeout(request.timer); request.reject(new HttpError(409, "device disconnected"));
  }
  for (const [sessionId, session] of sessions) if (session.deviceId === deviceId) sessions.delete(sessionId);
}

export async function syncWorkspaceAuthority(userId: string, workspaceId: string, clientId: string,
  transitions: Array<{ fromHash: string; generation: number; signature: string }>) {
  const proof = controlProof(userId, clientId); if (!proof) throw new HttpError(401, "control client authorization expired");
  const snapshot = canonicalAuthority(authoritySnapshot(workspaceId));
  const digest = new Bun.CryptoHasher("sha256").update(snapshot).digest("hex");
  const signatures = new Map<string, string>();
  for (const transition of transitions) {
    const fromHash = String(transition.fromHash || "").toLowerCase(), generation = Number(transition.generation), signature = String(transition.signature || "");
    const key = `${generation}:${fromHash}`;
    if (!/^[0-9a-f]{64}$/.test(fromHash) || !Number.isSafeInteger(generation) || generation < 0 || signatures.has(key)) {
      throw new HttpError(400, "invalid authority transition");
    }
    if (!await verifyClientSignature(userId, clientId, `rc-authority-v3\n${generation}\n${fromHash}\n${digest}`, signature)) {
      throw new HttpError(403, "invalid authority transition signature");
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
    const request = finishPending(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "challenge") return true;
    request.resolve({ challenge: message.challenge }); return true;
  }
  if (message.type === "control.ready") {
    const request = finishPending(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "open") return true;
    const clientId = request.clientId || "", iceServers = request.iceServers || [];
    sessions.set(message.sessionId, { userId: request.userId, clientId, deviceId, iceServers });
    request.resolve({ sessionId: message.sessionId, transportPublicKey: message.transportPublicKey,
      ephemeralPublicKey: message.ephemeralPublicKey, signature: message.signature, iceServers }); return true;
  }
  if (message.type === "control.webrtc.ready") {
    const request = finishPending(message.requestId); if (!request || request.deviceId !== deviceId || request.kind !== "webrtc") return true;
    request.resolve({ sdp: message.sdp }); return true;
  }
  if (message.type === "control.error") {
    const requestId = message.requestId || "", request = finishPending(requestId); if (!request) return true;
    request.reject(new HttpError(409, message.output || "control request rejected")); return true;
  }
  if (message.type === "control.closed") {
    const session = sessions.get(message.sessionId);
    if (session?.deviceId === deviceId) sessions.delete(message.sessionId);
    return true;
  }
  if (message.type === "lock.state") {
    q("UPDATE devices SET lock_hash=?,lock_generation=? WHERE id=?")
      .run(String(message.lockHash || "").slice(0, 64), Number(message.lockGeneration || 0), deviceId); return true;
  }
  return false;
}
