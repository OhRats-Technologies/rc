import { Elysia } from "elysia";
import { apiTokenScopes, auth } from "../auth";
import { browserSocketHandlers } from "../browser-socket";
import { PUBLIC_URL } from "../config";
import { fail } from "../http-utils";
import { BrowserCommandSchema } from "../protocol";

type Connection = ReturnType<typeof browserSocketHandlers.open>;
const connections = new WeakMap<object, Connection>();

export const browserSocketRoute = new Elysia({ name: "rc.websocket.browser", detail: { hide: true } })
  .derive(async ({ request }) => ({ rcUser: await auth(request) }))
  .ws("/api/v1/ws", {
    body: BrowserCommandSchema,
    beforeHandle({ request, rcUser }) {
      const origin = request.headers.get("origin");
      if (origin && origin !== new URL(request.url).origin && origin !== PUBLIC_URL) return fail("invalid origin", 403);
      if (!rcUser) return fail("authentication required", 401);
      const scopes = apiTokenScopes(request);
      if (scopes && !scopes.includes("execute")) return fail("API key requires execute scope", 403);
    },
    open(ws) {
      const user = ws.data.rcUser;
      if (!user) return ws.close(1008, "authentication required");
      connections.set(ws.raw, browserSocketHandlers.open(user.id, ws.raw, apiTokenScopes(ws.data.request)));
    },
    message(ws, message) {
      const user = ws.data.rcUser, connection = connections.get(ws.raw);
      if (!user || !connection) return;
      browserSocketHandlers.message(user.id, connection, message);
    },
    close(ws) {
      const connection = connections.get(ws.raw);
      if (connection) browserSocketHandlers.close(connection);
      connections.delete(ws.raw);
    },
  });
