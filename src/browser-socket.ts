import { deviceRole, logEvent } from "./core";
import { subscribeEvents } from "./events";
import { isOnline, sendNodeUpdate } from "./gateway";
import { inputProcess, resizeRemoteProcess, signalProcess, startProcess } from "./process-api";
import { workspaceForDevice } from "./process-store";
import type { BrowserCommand, BrowserServerMessage } from "./protocol";

export type SocketWriter = { send(data: string): unknown; close(code?: number, reason?: string): void };
type BrowserConnection = { socket: SocketWriter; unsubscribe?: () => void };

function send(connection: BrowserConnection, value: BrowserServerMessage) {
  try { connection.socket.send(JSON.stringify(value)); } catch {}
}

function updateNode(userId: string, input: any) {
  const deviceId = String(input.deviceId || ""), role = deviceRole(userId, deviceId);
  if (role !== "owner") throw new Error("owner required");
  if (!isOnline(deviceId)) throw new Error("device is offline");
  if (!sendNodeUpdate(deviceId)) throw new Error("node does not support remote update");
  logEvent("node.update.requested", workspaceForDevice(deviceId), userId, deviceId);
  return { ok: true };
}

export const browserSocketHandlers = {
  open(userId: string, socket: SocketWriter) {
    const connection: BrowserConnection = { socket };
    connection.unsubscribe = subscribeEvents(userId, event => send(connection, { type: "event", event }));
    send(connection, { type: "ready" });
    return connection;
  },
  message(userId: string, connection: BrowserConnection, message: BrowserCommand) {
    const requestId = "requestId" in message ? String(message.requestId || "") : "";
    try {
      let result: unknown;
      switch (message.type) {
        case "ping": send(connection, { type: "pong" }); return;
        case "process.start": result = startProcess(userId, message); break;
        case "process.input": result = inputProcess(userId, message); break;
        case "process.resize": result = resizeRemoteProcess(userId, message); break;
        case "process.signal": result = signalProcess(userId, message); break;
        case "node.update": result = updateNode(userId, message); break;
      }
      if (requestId) send(connection, { type: "response", requestId, ok: true, result });
    } catch (error) {
      if (requestId) send(connection, { type: "response", requestId, ok: false,
        error: error instanceof Error ? error.message : "request failed" });
    }
  },
  close(connection: BrowserConnection) {
    connection.unsubscribe?.();
  },
};
