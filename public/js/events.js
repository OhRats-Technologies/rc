let socket = null;
let ready = false;
let closed = false;
let reconnectTimer = null;
let heartbeat = null;
const listeners = new Set();
const pending = new Map();
const waiters = [];

function emit(event) {
  for (const listener of listeners) listener(event);
}

function connect() {
  if (closed || socket && [WebSocket.OPEN, WebSocket.CONNECTING].includes(socket.readyState)) return;
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
  socket = new WebSocket(`${scheme}//${location.host}/api/v1/ws`);
  socket.onmessage = message => {
    let value;
    try { value = JSON.parse(message.data); } catch { return; }
    if (value.type === 'ready') {
      ready = true;
      clearInterval(heartbeat);
      heartbeat = setInterval(() => { if (socket?.readyState === WebSocket.OPEN) socket.send('{"type":"ping"}'); }, 20000);
      while (waiters.length) waiters.shift()();
      emit({ kind: 'relay.connected' });
      return;
    }
    if (value.type === 'event') { emit(value.event); return; }
    if (value.type === 'response' && pending.has(value.requestId)) {
      const request = pending.get(value.requestId); pending.delete(value.requestId);
      clearTimeout(request.timer);
      value.ok ? request.resolve(value.result) : request.reject(new Error(value.error || 'request failed'));
    }
  };
  socket.onclose = () => {
    ready = false;
    clearInterval(heartbeat); heartbeat = null;
    for (const request of pending.values()) { clearTimeout(request.timer); request.reject(new Error('Relay connection closed')); }
    pending.clear();
    if (!closed) reconnectTimer = setTimeout(connect, 1000);
  };
}

export function startEvents() { connect(); }

async function whenReady() {
  connect();
  if (ready && socket?.readyState === WebSocket.OPEN) return;
  await new Promise(resolve => waiters.push(resolve));
}

export async function relayRequest(type, payload = {}) {
  await whenReady();
  const requestId = crypto.randomUUID();
  const result = new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(requestId);
      reject(new Error('Relay request timed out'));
    }, 15000);
    pending.set(requestId, { resolve, reject, timer });
  });
  socket.send(JSON.stringify({ type, requestId, ...payload }));
  return result;
}

export function relaySend(type, payload = {}) {
  if (!ready || socket?.readyState !== WebSocket.OPEN) return false;
  socket.send(JSON.stringify({ type, ...payload }));
  return true;
}

export function onRelayEvent(listener) {
  connect(); listeners.add(listener);
  return () => listeners.delete(listener);
}

addEventListener('pagehide', () => {
  closed = true; clearTimeout(reconnectTimer); clearInterval(heartbeat); socket?.close();
}, { once: true });
