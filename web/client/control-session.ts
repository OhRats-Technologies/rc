import { api } from "./http";
import { request } from "./socket";
import { b64urlToBytes, bytesToB64url, ensureControlAuthorized, pinDevice } from "./control-client";
import { websocketControlTransport, type ControlTransport } from "./control-transport";
import type { Device } from "../types";

type SessionMessage = { type: string; [key: string]: unknown };
type Listener = (message: SessionMessage) => void;

function pemBytes(value: string) {
  const base64 = value.replace(/-----BEGIN PUBLIC KEY-----|-----END PUBLIC KEY-----|\s/g, "");
  return Uint8Array.from(atob(base64), char => char.charCodeAt(0));
}
function nonce(direction: number, sequence: number) {
  const value = new Uint8Array(12); value[0] = direction;
  new DataView(value.buffer).setBigUint64(4, BigInt(sequence)); return value;
}
function aad(sessionId: string, sequence: number, direction: string) {
  return new TextEncoder().encode(`rc-frame-v1\n${sessionId}\n${sequence}\n${direction}`);
}
function sessionPayload(challenge: string, deviceId: string, clientId: string, publicKey: string) {
  return `rc-session-v1\n${challenge}\n${deviceId}\n${clientId}\n${publicKey}`;
}
function readyPayload(challenge: string, deviceId: string, clientId: string, publicKey: string, transportKey: string, ephemeralKey: string, sessionId: string) {
  return `rc-ready-v2\n${challenge}\n${deviceId}\n${clientId}\n${publicKey}\n${transportKey}\n${ephemeralKey}\n${sessionId}`;
}

async function sharedSecret(privateKey: CryptoKey, publicKey: string) {
  const remote = await crypto.subtle.importKey("raw", b64urlToBytes(publicKey), { name: "X25519" }, false, []);
  return new Uint8Array(await crypto.subtle.deriveBits({ name: "X25519", public: remote }, privateKey, 256));
}

async function deriveKey(privateKey: CryptoKey, transportKey: string, ephemeralKey: string, challenge: string, deviceId: string, clientId: string) {
  const staticShared = await sharedSecret(privateKey, transportKey), ephemeralShared = await sharedSecret(privateKey, ephemeralKey);
  const combined = new Uint8Array(staticShared.length + ephemeralShared.length); combined.set(staticShared); combined.set(ephemeralShared, staticShared.length);
  const material = await crypto.subtle.importKey("raw", combined, "HKDF", false, ["deriveKey"]);
  const salt = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(challenge));
  return await crypto.subtle.deriveKey({ name: "HKDF", hash: "SHA-256", salt, info: new TextEncoder().encode(`rc-e2e-v2\n${deviceId}\n${clientId}`) },
    material, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

export class ControlSession {
  private sendSequence = 0;
  private receiveSequence = 0;
  private listeners = new Set<Listener>();
  private pending = new Map<string, { resolve: (value: SessionMessage) => void; reject: (error: Error) => void; timer: number }>();
  private unsubscribe: () => void;
  constructor(readonly deviceId: string, readonly sessionId: string, private key: CryptoKey, private transport: ControlTransport) {
    this.unsubscribe = transport.onFrame((sequence, ciphertext) => { void this.receive(sequence, ciphertext); });
  }
  onMessage(listener: Listener) { this.listeners.add(listener); return () => this.listeners.delete(listener); }
  async send(message: SessionMessage) {
    const sequence = ++this.sendSequence, plain = new TextEncoder().encode(JSON.stringify(message));
    const encrypted = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce(1, sequence), additionalData: aad(this.sessionId, sequence, "c2n") }, this.key, plain);
    if (!this.transport.send(sequence, bytesToB64url(encrypted))) throw new Error("RC connection unavailable");
  }
  async request(message: SessionMessage) {
    const requestId = crypto.randomUUID(), result = new Promise<SessionMessage>((resolve, reject) => {
      const timer = window.setTimeout(() => { this.pending.delete(requestId); reject(new Error("Node request timed out")); }, 15_000);
      this.pending.set(requestId, { resolve, reject, timer });
    });
    await this.send({ ...message, requestId }); return await result;
  }
  private async receive(sequence: number, ciphertext: string) {
    if (sequence !== this.receiveSequence + 1) { this.close(); return; }
    try {
      const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: nonce(2, sequence), additionalData: aad(this.sessionId, sequence, "n2c") }, this.key, b64urlToBytes(ciphertext));
      this.receiveSequence = sequence; const message = JSON.parse(new TextDecoder().decode(plain)) as SessionMessage;
      if (message.type === "control.revoked") { this.close(); return; }
      const requestId = String(message.requestId || ""), pending = this.pending.get(requestId);
      if (pending && message.type === "control.result") {
        clearTimeout(pending.timer); this.pending.delete(requestId);
        if (String(message.output || "") === "ok") pending.resolve(message); else pending.reject(new Error(String(message.output || "Node request failed")));
      }
      for (const listener of this.listeners) listener(message);
    } catch { this.close(); }
  }
  close() {
    this.unsubscribe(); this.transport.close(); this.listeners.clear();
    for (const pending of this.pending.values()) { clearTimeout(pending.timer); pending.reject(new Error("Control session closed")); }
    this.pending.clear();
  }
}

export async function openControlSession(deviceId: string) {
  const identity = await ensureControlAuthorized();
  const { device } = await api<{ device: Device & { identity_public_key?: string; transport_public_key?: string } }>(`/api/v1/devices/${encodeURIComponent(deviceId)}`);
  const identityKey = String(device.identity_public_key || ""), expectedTransport = String(device.transport_public_key || "");
  if (!identityKey || !expectedTransport) throw new Error("Update this RC Node before opening an encrypted control session.");
  await pinDevice(deviceId, identityKey, expectedTransport);
  const { challenge } = await request<{ challenge: string }>({ type: "control.challenge", deviceId });
  const pair = await crypto.subtle.generateKey({ name: "X25519" }, true, ["deriveBits"]);
  const publicKey = bytesToB64url(await crypto.subtle.exportKey("raw", pair.publicKey));
  const payload = sessionPayload(challenge, deviceId, identity.id, publicKey);
  const signature = bytesToB64url(await crypto.subtle.sign("Ed25519", identity.privateKey, new TextEncoder().encode(payload)));
  const ready = await request<{ sessionId: string; transportPublicKey: string; ephemeralPublicKey: string; signature: string }>({
    type: "control.open", deviceId, challenge, clientId: identity.id, publicKey, signature,
  });
  if (ready.transportPublicKey !== expectedTransport || !ready.ephemeralPublicKey) throw new Error("RC Node transport identity changed.");
  const deviceKey = await crypto.subtle.importKey("spki", pemBytes(identityKey), { name: "Ed25519" }, false, ["verify"]);
  const verified = await crypto.subtle.verify("Ed25519", deviceKey, b64urlToBytes(ready.signature),
    new TextEncoder().encode(readyPayload(challenge, deviceId, identity.id, publicKey, ready.transportPublicKey, ready.ephemeralPublicKey, ready.sessionId)));
  if (!verified) throw new Error("RC Node handshake signature failed.");
  return new ControlSession(deviceId, ready.sessionId,
    await deriveKey(pair.privateKey, ready.transportPublicKey, ready.ephemeralPublicKey, challenge, deviceId, identity.id),
    websocketControlTransport(deviceId, ready.sessionId));
}

