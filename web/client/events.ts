import type { RCEvent } from "../types";

type Listener = (event: RCEvent) => void;
const listeners = new Set<Listener>();
let source: EventSource | null = null, closed = false;

function emit(event: RCEvent) { for (const listener of listeners) listener(event); }
function connect() {
  if (closed || source) return;
  source = new EventSource("/api/v1/events");
  source.onopen = () => emit({ kind: "rc.connected" });
  source.onmessage = event => {
    try { emit(JSON.parse(event.data) as RCEvent); } catch {}
  };
  source.onerror = () => { /* EventSource reconnects automatically. */ };
}

export function onEvent(listener: Listener) {
  connect(); listeners.add(listener); return () => listeners.delete(listener);
}

addEventListener("pagehide", () => {
  closed = true; source?.close(); source = null;
}, { once: true });
