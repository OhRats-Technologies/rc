import { Elysia } from "elysia";
import { auth } from "../auth";
import { browserSocketHandlers } from "../browser-socket";
import { PUBLIC_URL } from "../config";
import { fail } from "../http-utils";
import { BrowserCommandSchema } from "../protocol";

type Connection = ReturnType<typeof browserSocketHandlers.open>;
const connections = new WeakMap<object, Connection>();

export const browserSocketRoute = new Elysia({ name: "relay.websocket.browser" })
  .derive(async ({ request }) => ({ relayUser: await auth(request) }))
  .ws("/api/v1/ws", {
    body: BrowserCommandSchema,
    beforeHandle({ request, relayUser }) {
      const origin = request.headers.get("origin");
      if (origin && origin !== new URL(request.url).origin && origin !== PUBLIC_URL) return fail("invalid origin", 403);
      if (!relayUser) return fail("authentication required", 401);
    },
    open(ws) {
      const user = ws.data.relayUser;
      if (!user) return ws.close(1008, "authentication required");
      connections.set(ws.raw, browserSocketHandlers.open(user.id, ws.raw));
    },
    message(ws, message) {
      const user = ws.data.relayUser, connection = connections.get(ws.raw);
      if (!user || !connection) return;
      browserSocketHandlers.message(user.id, connection, message);
    },
    close(ws) {
      const connection = connections.get(ws.raw);
      if (connection) browserSocketHandlers.close(connection);
      connections.delete(ws.raw);
    },
  });
