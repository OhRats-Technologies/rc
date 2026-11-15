import { closeControlSession } from "./control-api";
import type { ControlTransport } from "./control-transport";

type Frame = { type: "control.frame"; sessionId: string; sequence: number; ciphertext: string };

export type ControlTransportStatus = {
  transport: "websocket";
  phase?: "connecting" | "connected" | "failed";
  reason?: string;
};

type StatusReporter = (status: ControlTransportStatus) => void;

export function controlPipePath(sessionId: string) {
  return `/api/v1/control/pipe/${encodeURIComponent(sessionId)}`;
}

export function controlPipeUrl(sessionId: string, origin: { protocol: string; host: string }) {
  const scheme = origin.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${origin.host}${controlPipePath(sessionId)}`;
}

function waitForOpen(socket: WebSocket, maxWaitMs: number) {
  if (socket.readyState === WebSocket.OPEN) return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => { cleanup(); reject(new Error("Control pipe timed out")); }, maxWaitMs);
    const ready = () => { cleanup(); resolve(); };
    const failed = () => { cleanup(); reject(new Error("Control pipe unavailable")); };
    const closed = () => { cleanup(); reject(new Error("Control pipe closed")); };
    const cleanup = () => {
      clearTimeout(timer);
      socket.removeEventListener("open", ready);
      socket.removeEventListener("error", failed);
      socket.removeEventListener("close", closed);
    };
    socket.addEventListener("open", ready);
    socket.addEventListener("error", failed);
    socket.addEventListener("close", closed);
  });
}

export async function openWebSocketControlTransport(
  sessionId: string, onStatus: StatusReporter = () => {},
): Promise<ControlTransport> {
  if (typeof WebSocket === "undefined") throw new Error("WebSocket unavailable in this browser");
  const listeners = new Set<(sequence: number, ciphertext: string) => void>();
  let closing = false, established = false;
  const publish = (status: ControlTransportStatus) => onStatus(status);
  const socket = new WebSocket(controlPipeUrl(sessionId, location));
  publish({ transport: "websocket", phase: "connecting" });

  const fail = (reason: string) => {
    if (closing) return;
    closing = true;
    publish({ transport: "websocket", phase: "failed", reason: reason.slice(0, 200) });
    try { socket.close(); } catch {}
    closeControlSession(sessionId);
  };

  socket.addEventListener("message", event => {
    if (closing) return;
    if (typeof event.data !== "string") { fail("Invalid control frame"); return; }
    if (event.data.length > 2_000_000) { fail("Invalid control frame"); return; }
    let frame: Frame;
    try { frame = JSON.parse(event.data) as Frame; } catch { fail("Invalid control frame"); return; }
    if (frame.type !== "control.frame" || frame.sessionId !== sessionId) return;
    for (const listener of listeners) listener(Number(frame.sequence), String(frame.ciphertext || ""));
  });
  socket.addEventListener("close", () => {
    if (!closing && established) fail("Control pipe closed");
  });

  try {
    await waitForOpen(socket, 15_000);
  } catch (error) {
    socket.close();
    throw error instanceof Error ? error : new Error("Control pipe unavailable");
  }
  established = true;
  publish({ transport: "websocket", phase: "connected" });

  return {
    send(sequence, ciphertext) {
      if (closing || socket.readyState !== WebSocket.OPEN) return false;
      try {
        socket.send(JSON.stringify({ type: "control.frame", sessionId, sequence, ciphertext } satisfies Frame));
        return true;
      } catch {
        fail("Control pipe send failed");
        return false;
      }
    },
    onFrame(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    close() {
      if (closing) return;
      closing = true;
      try { socket.close(); } catch {}
      closeControlSession(sessionId);
    },
  };
}
