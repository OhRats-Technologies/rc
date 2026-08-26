import { connect, type Socket } from "node:net";
import { Elysia } from "elysia";
import { SSH_DAEMON_PORT } from "../config";

const tunnels = new WeakMap<object, Socket>();

export const sshTunnelRoute = new Elysia({ name: "rc.websocket.ssh", detail: { hide: true } })
  .ws("/api/v1/ssh/tunnel", {
    open(ws) {
      const socket = connect({ host: "127.0.0.1", port: SSH_DAEMON_PORT });
      tunnels.set(ws.raw, socket);
      socket.on("data", data => { try { ws.raw.sendBinary(Buffer.from(data)); } catch { socket.destroy(); } });
      socket.on("end", () => ws.close(1000, "SSH edge closed"));
      socket.on("error", () => ws.close(1011, "SSH edge unavailable"));
      socket.on("close", () => tunnels.delete(ws.raw));
    },
    message(ws, message) {
      const socket = tunnels.get(ws.raw); if (!socket || socket.destroyed) return;
      if (typeof message === "string") return ws.close(1003, "binary SSH tunnel required");
      socket.write(Buffer.isBuffer(message) ? message : Buffer.from(message as ArrayBuffer));
    },
    close(ws) { tunnels.get(ws.raw)?.destroy(); tunnels.delete(ws.raw); },
  });
