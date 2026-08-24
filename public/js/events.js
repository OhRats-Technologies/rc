let source = null;
const listeners = new Set();

export function startEvents() {
  if (source) return;
  source = new EventSource('/api/v1/events');
  source.onmessage = message => {
    let event;
    try { event = JSON.parse(message.data); } catch { return; }
    for (const listener of listeners) listener(event);
  };
}

export function onRelayEvent(listener) {
  startEvents();
  listeners.add(listener);
  return () => listeners.delete(listener);
}

addEventListener('pagehide', () => source?.close(), { once: true });
