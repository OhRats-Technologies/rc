import { canWrite, deviceRole, logEvent } from "./core";
import type { ServerWebSocket } from "bun";
import { subscribeEvents } from "./events";
import { isOnline, sendNodeUpdate } from "./gateway";
import { inputProcess, resizeRemoteProcess, signalProcess, startProcess } from "./process-api";
import { workspaceForDevice } from "./process-store";

export type BrowserData = { kind: "browser"; userId: string; unsubscribe?: () => void };

function send(ws: ServerWebSocket<BrowserData>, value: unknown) {
  try { ws.send(JSON.stringify(value)); } catch {}
}

function updateNode(userId: string, input: any) {
  const deviceId = String(input.deviceId || ""), role = deviceRole(userId, deviceId);
  if (!canWrite(role)) throw new Error("forbidden");
  if (!isOnline(deviceId)) throw new Error("device is offline");
  if (!sendNodeUpdate(deviceId)) throw new Error("node does not support remote update");
  logEvent("node.update.requested", workspaceForDevice(deviceId), userId, deviceId);
  return { ok: true };
}

export const browserSocketHandlers = {
  open(ws: ServerWebSocket<BrowserData>) {
    ws.data.unsubscribe = subscribeEvents(ws.data.userId, event => send(ws, { type: "event", event }));
    send(ws, { type: "ready" });
  },
  message(ws: ServerWebSocket<BrowserData>, raw: string | Uint8Array) {
    let message: any;
    try { message = JSON.parse(typeof raw === "string" ? raw : new TextDecoder().decode(raw)); }
    catch { return; }
    const requestId = String(message.requestId || "");
    try {
      let result: unknown;
      switch (message.type) {
        case "ping": send(ws, { type: "pong" }); return;
        case "process.start": result = startProcess(ws.data.userId, message); break;
        case "process.input": result = inputProcess(ws.data.userId, message); break;
        case "process.resize": result = resizeRemoteProcess(ws.data.userId, message); break;
        case "process.signal": result = signalProcess(ws.data.userId, message); break;
        case "node.update": result = updateNode(ws.data.userId, message); break;
        default: throw new Error("unknown realtime command");
      }
      if (requestId) send(ws, { type: "response", requestId, ok: true, result });
    } catch (error: any) {
      if (requestId) send(ws, { type: "response", requestId, ok: false, error: error?.message || "request failed" });
    }
  },
  close(ws: ServerWebSocket<BrowserData>) {
    ws.data.unsubscribe?.();
  },
};
