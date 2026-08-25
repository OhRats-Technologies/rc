import type { BrowserCommand } from "../../src/protocol";
import type { RCEvent } from "../types";

type Listener = (event: RCEvent) => void;
type ControlFrame = { sessionId: string; sequence: number; ciphertext: string };
type FrameListener = (frame: ControlFrame) => void;
type Pending = { resolve: (value: unknown) => void; reject: (error: Error) => void; timer: number };
let socket: WebSocket | null = null, ready = false, closed = false, reconnectTimer = 0, heartbeat = 0;
const listeners = new Set<Listener>(), frameListeners = new Set<FrameListener>(), pending = new Map<string, Pending>(), waiters = new Set<() => void>();
type WithoutRequestId<T> = T extends unknown ? Omit<T, "requestId"> : never;
type RequestCommand = WithoutRequestId<Exclude<BrowserCommand, { type: "ping" }>>;

function emit(event: RCEvent) { for (const listener of listeners) listener(event); }
function connect() {
  if (closed || socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) return;
  socket = new WebSocket(`${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/v1/ws`);
  socket.onmessage = ({ data }) => {
    let value: { type: string; [key: string]: unknown }; try { value = JSON.parse(String(data)); } catch { return; }
    if (value.type === "ready") {
      ready = true; clearInterval(heartbeat);
      heartbeat = window.setInterval(() => send({ type: "ping" }), 20_000);
      for (const waiter of waiters) waiter(); waiters.clear(); emit({ kind: "rc.connected" }); return;
    }
    if (value.type === "event") { emit(value.event as RCEvent); return; }
    if (value.type === "control.frame") {
      const frame = value as unknown as ControlFrame; for (const listener of frameListeners) listener(frame); return;
    }
    if (value.type !== "response") return;
    const requestId = String(value.requestId || ""), request = pending.get(requestId); if (!request) return;
    clearTimeout(request.timer); pending.delete(requestId);
    value.ok ? request.resolve(value.result) : request.reject(new Error(String(value.error || "request failed")));
  };
  socket.onclose = () => {
    ready = false; clearInterval(heartbeat);
    for (const request of pending.values()) { clearTimeout(request.timer); request.reject(new Error("RC connection closed")); }
    pending.clear(); if (!closed) reconnectTimer = window.setTimeout(connect, 1000);
  };
}

async function whenReady() {
  connect(); if (ready && socket?.readyState === WebSocket.OPEN) return;
  await new Promise<void>(resolve => waiters.add(resolve));
}

function send(message: BrowserCommand) {
  if (!ready || socket?.readyState !== WebSocket.OPEN) return false;
  socket.send(JSON.stringify(message)); return true;
}

export async function request<T>(message: RequestCommand): Promise<T> {
  await whenReady(); const requestId = crypto.randomUUID();
  const result = new Promise<unknown>((resolve, reject) => {
    const timer = window.setTimeout(() => { pending.delete(requestId); reject(new Error("RC request timed out")); }, 10_000);
    pending.set(requestId, { resolve, reject, timer });
  });
  socket?.send(JSON.stringify({ ...message, requestId }));
  return await result as T;
}

export function fire(message: BrowserCommand) { connect(); return send(message); }
export function onEvent(listener: Listener) { connect(); listeners.add(listener); return () => listeners.delete(listener); }
export function onControlFrame(listener: FrameListener) { connect(); frameListeners.add(listener); return () => frameListeners.delete(listener); }

addEventListener("pagehide", () => {
  closed = true; clearTimeout(reconnectTimer); clearInterval(heartbeat); socket?.close();
}, { once: true });
