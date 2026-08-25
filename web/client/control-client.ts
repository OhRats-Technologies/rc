import { api } from "./http";
import { passkeyAssertion } from "./webauthn";
import { request } from "./socket";
import type { Me } from "../types";

type ControlIdentity = { id: string; publicKey: string; privateKey: CryptoKey };
type DevicePin = { identityKey: string; transportKey: string };
const DB_NAME = "ohrats-rc-security", STORE = "keys";

export function bytesToB64url(bytes: ArrayBuffer | Uint8Array) {
  const value = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = ""; for (const byte of value) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

export function b64urlToBytes(value: string) {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - value.length % 4) % 4);
  return Uint8Array.from(atob(base64), char => char.charCodeAt(0));
}

function db() {
  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(STORE);
    request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error);
  });
}

async function stored<T>(key: string): Promise<T | null> {
  const database = await db();
  return await new Promise((resolve, reject) => {
    const request = database.transaction(STORE).objectStore(STORE).get(key);
    request.onsuccess = () => resolve(request.result || null); request.onerror = () => reject(request.error);
  });
}

async function save(key: string, value: unknown) {
  const database = await db();
  await new Promise<void>((resolve, reject) => {
    const request = database.transaction(STORE, "readwrite").objectStore(STORE).put(value, key);
    request.onsuccess = () => resolve(); request.onerror = () => reject(request.error);
  });
}

async function createIdentity(): Promise<ControlIdentity> {
  const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
  const publicKey = bytesToB64url(await crypto.subtle.exportKey("raw", pair.publicKey));
  const pkcs8 = await crypto.subtle.exportKey("pkcs8", pair.privateKey);
  const privateKey = await crypto.subtle.importKey("pkcs8", pkcs8, { name: "Ed25519" }, false, ["sign"]);
  const identity = { id: crypto.randomUUID(), publicKey, privateKey };
  await save("control-identity", identity); return identity;
}

export async function controlIdentity() {
  return await stored<ControlIdentity>("control-identity") || await createIdentity();
}

export async function ensureControlAuthorized() {
  const identity = await controlIdentity();
  const status = await api<{ authorized: boolean; expiresAt?: number }>(`/api/v1/control/clients/${encodeURIComponent(identity.id)}`);
  if (status.authorized && Number(status.expiresAt || 0) > Date.now() + 60_000) return identity;
  const start = await api<{ authorizationId: string; options: any }>("/api/v1/control/authorize/options", {
    method: "POST", body: JSON.stringify({ clientId: identity.id, signingPublicKey: identity.publicKey }),
  });
  const response = await passkeyAssertion(start.options);
  await api("/api/v1/control/authorize/verify", {
    method: "POST", body: JSON.stringify({ authorizationId: start.authorizationId, response }),
  });
  return identity;
}

export async function signControl(payload: string) {
  const identity = await ensureControlAuthorized();
  const signature = await crypto.subtle.sign("Ed25519", identity.privateKey, new TextEncoder().encode(payload));
  return { clientId: identity.id, signature: bytesToB64url(signature) };
}

export async function syncWorkspaceAuthority(workspaceId: string) {
  const state = await api<{ hash: string; devices: number; synced: number }>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/authority`);
  if (!state.devices || state.synced === state.devices) return state;
  const identity = await ensureControlAuthorized();
  const signature = bytesToB64url(await crypto.subtle.sign("Ed25519", identity.privateKey,
    new TextEncoder().encode(`rc-authority-v1\n${state.hash}`)));
  await request({ type: "lock.sync", workspaceId, clientId: identity.id, signature });
  for (let attempt = 0; attempt < 15; attempt++) {
    await new Promise(resolve => window.setTimeout(resolve, 200));
    const next = await api<{ hash: string; devices: number; synced: number }>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/authority`);
    if (!next.devices || next.synced === next.devices) return next;
  }
  const final = await api<{ hash: string; devices: number; synced: number }>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/authority`);
  throw new Error(`RC Lock sync was rejected or timed out (${final.synced}/${final.devices} Nodes accepted it).`);
}

export async function syncOwnedAuthorities() {
  const me = await api<Me>("/api/v1/me");
  for (const workspace of me.workspaces.filter(item => item.role === "owner")) await syncWorkspaceAuthority(workspace.id);
}

export async function pinDevice(deviceId: string, identityKey: string, transportKey: string) {
  const key = `device:${deviceId}`, current = await stored<DevicePin>(key);
  if (current && (current.identityKey !== identityKey || current.transportKey !== transportKey)) {
    throw new Error("RC Node cryptographic identity changed. Re-enroll the device before trusting it again.");
  }
  if (!current) await save(key, { identityKey, transportKey });
}

