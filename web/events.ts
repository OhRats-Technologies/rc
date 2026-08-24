import type { RelayEvent } from "./types";

type Listener = (event: RelayEvent) => void;
type Pending = { resolve: (value: unknown) => void; reject: (error: Error) => void; timer: number };

let socket: WebSocket | null = null;
let ready = false;
let closed = false;
let reconnectTimer = 0;
let heartbeat = 0;
const listeners = new Set<Listener>();
const pending = new Map<string, Pending>();
const waiters = new Set<() => void>();

function emit(event: RelayEvent) { for (const listener of listeners) listener(event); }

function connect() {
  if (closed || socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) return;
  socket = new WebSocket(`${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/v1/ws`);
  socket.onmessage = ({ data }) => {
    let value: any;
    try { value = JSON.parse(String(data)); } catch { return; }
    if (value.type === "ready") {
      ready = true;
      clearInterval(heartbeat);
      heartbeat = window.setInterval(() => socket?.readyState === WebSocket.OPEN && socket.send('{"type":"ping"}'), 20_000);
      for (const waiter of waiters) waiter();
      waiters.clear();
      emit({ kind: "relay.connected" });
      return;
    }
    if (value.type === "event") { emit(value.event as RelayEvent); return; }
    if (value.type !== "response") return;
    const request = pending.get(String(value.requestId));
    if (!request) return;
    clearTimeout(request.timer);
    pending.delete(String(value.requestId));
    value.ok ? request.resolve(value.result) : request.reject(new Error(value.error || "request failed"));
  };
  socket.onclose = () => {
    ready = false;
    clearInterval(heartbeat);
    for (const request of pending.values()) {
      clearTimeout(request.timer);
      request.reject(new Error("Relay connection closed"));
    }
    pending.clear();
    if (!closed) reconnectTimer = window.setTimeout(connect, 1000);
  };
}

async function whenReady() {
  connect();
  if (ready && socket?.readyState === WebSocket.OPEN) return;
  await new Promise<void>(resolve => waiters.add(resolve));
}

export function startEvents() { connect(); }

export async function relayRequest<T>(type: string, payload: Record<string, unknown> = {}): Promise<T> {
  await whenReady();
  const requestId = crypto.randomUUID();
  const result = new Promise<unknown>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      pending.delete(requestId);
      reject(new Error("Relay request timed out"));
    }, 10_000);
    pending.set(requestId, { resolve, reject, timer });
  });
  socket?.send(JSON.stringify({ type, requestId, ...payload }));
  return await result as T;
}

export function relaySend(type: string, payload: Record<string, unknown> = {}) {
  if (!ready || socket?.readyState !== WebSocket.OPEN) return false;
  socket.send(JSON.stringify({ type, ...payload }));
  return true;
}

export function onRelayEvent(listener: Listener) {
  connect();
  listeners.add(listener);
  return () => listeners.delete(listener);
}

addEventListener("pagehide", () => {
  closed = true;
  clearTimeout(reconnectTimer);
  clearInterval(heartbeat);
  socket?.close();
}, { once: true });
