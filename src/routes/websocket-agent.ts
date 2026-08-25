import { Elysia, t } from "elysia";
import { q } from "../db";
import { agentSocketHandlers, verifyAgent } from "../gateway";
import { fail } from "../http-utils";
import { AgentClientMessageSchema } from "../protocol";

const AgentQuery = t.Object({
  device: t.String({ minLength: 1, maxLength: 100 }),
});

export const agentSocketRoute = new Elysia({ name: "rc.websocket.agent", detail: { hide: true } })
  .ws("/api/v1/agent/ws", {
    query: AgentQuery,
    body: AgentClientMessageSchema,
    async beforeHandle({ request, query }) {
      if (!q("SELECT 1 FROM devices WHERE id=?").get(query.device)) return fail("device not found", 404);
      if (await verifyAgent(request, query.device) !== query.device) return fail("invalid agent signature", 401);
    },
    open(ws) { agentSocketHandlers.open(ws.data.query.device, ws.raw); },
    message(ws, message) { agentSocketHandlers.message(ws.data.query.device, message); },
    close(ws) { agentSocketHandlers.close(ws.data.query.device, ws.raw); },
  });
