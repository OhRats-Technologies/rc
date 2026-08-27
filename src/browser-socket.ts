import { subscribeEvents } from "./events";
import { allocateProcess } from "./process-api";
import type { BrowserCommand, BrowserServerMessage } from "./protocol";
import type { ApiScope } from "./account";
import {
  closeControlSession, relayControlFrame, releaseControlSocket, reportControlTransport, requestControlChallenge, requestControlOpen, requestControlWebRTC,
  syncWorkspaceAuthority,
} from "./control-relay";

export type SocketWriter = { send(data: string): unknown; close(code?: number, reason?: string): void };
type BrowserConnection = { socket: SocketWriter; scopes: ApiScope[] | null; apiKeyId: string | null; unsubscribe?: () => void };

function requireScope(connection: BrowserConnection, scope: ApiScope) {
  if (connection.scopes && !connection.scopes.includes(scope)) throw new Error(`API key requires ${scope} scope`);
}

function requireControlScope(connection: BrowserConnection) {
  if (connection.scopes && !connection.scopes.some(scope => scope === "execute" || scope === "manage-devices")) {
    throw new Error("API key requires execute or manage-devices scope");
  }
}

function send(connection: BrowserConnection, value: BrowserServerMessage) {
  try { connection.socket.send(JSON.stringify(value)); } catch {}
}

export const browserSocketHandlers = {
  open(userId: string, socket: SocketWriter, scopes: ApiScope[] | null = null, apiKeyId: string | null = null) {
    const connection: BrowserConnection = { socket, scopes, apiKeyId };
    connection.unsubscribe = subscribeEvents(userId, event => send(connection, { type: "event", event }));
    send(connection, { type: "ready" });
    return connection;
  },
  async message(userId: string, connection: BrowserConnection, message: BrowserCommand) {
    const requestId = "requestId" in message ? String(message.requestId || "") : "";
    try {
      let result: unknown;
      switch (message.type) {
        case "ping": send(connection, { type: "pong" }); return;
        case "process.allocate": requireScope(connection, "execute"); result = allocateProcess(userId, { ...message, origin: "browser" }); break;
        case "control.challenge": requireControlScope(connection); requestControlChallenge(userId, message.deviceId, requestId, connection.socket); return;
        case "control.open": requireControlScope(connection); await requestControlOpen(userId, message, connection.socket, connection.apiKeyId); return;
        case "control.webrtc": requireControlScope(connection); requestControlWebRTC(userId, message, connection.socket); return;
        case "control.transport": requireControlScope(connection); reportControlTransport(userId, message, connection.socket); return;
        case "control.frame": requireControlScope(connection); relayControlFrame(userId, message, connection.socket); return;
        case "control.close": closeControlSession(message, connection.socket); return;
        case "lock.sync": result = await syncWorkspaceAuthority(userId, message.workspaceId, message.clientId, message.transitions); break;
      }
      if (requestId) send(connection, { type: "response", requestId, ok: true, result });
    } catch (error) {
      if (requestId) send(connection, { type: "response", requestId, ok: false,
        error: error instanceof Error ? error.message : "request failed" });
    }
  },
  close(connection: BrowserConnection) {
    connection.unsubscribe?.();
    releaseControlSocket(connection.socket);
  },
};
