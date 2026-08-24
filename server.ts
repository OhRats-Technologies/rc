import app from "./web/index.html";
import type { ServerWebSocket } from "bun";
import { app as elysia } from "./src/app";
import { PUBLIC_URL, PORT, SETUP_TOKEN, VERSION } from "./src/config";
import { auth } from "./src/auth";
import { BrowserData, browserSocketHandlers } from "./src/browser-socket";
import { now, q, sha } from "./src/db";
import { AgentData, agentSocketHandlers, agentsCount, recoverInterruptedProcesses, verifyAgent } from "./src/gateway";
import { fail, json, setupCookie } from "./src/http-utils";
import { download, frontendAsset, frontendHTML, installScript } from "./src/artifacts";

recoverInterruptedProcesses();

type SocketData = AgentData | BrowserData;
const production = Bun.env.NODE_ENV === "production";
const spa = production ? () => frontendHTML(import.meta.dir) : app;

function setupRoute(req: Request & { params: { token: string } }) {
  if ((q<any>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return Response.redirect(PUBLIC_URL + "/", 303);
  const token = req.params.token || "";
  if (!SETUP_TOKEN || sha(token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
  return new Response(null, {
    status: 303,
    headers: { location: "/", "set-cookie": setupCookie(token), "cache-control": "no-store" },
  });
}

const server = Bun.serve<SocketData>({
  port: PORT,
  hostname: "0.0.0.0",
  idleTimeout: 60,
  development: Bun.env.NODE_ENV === "development",
  routes: {
    "/": spa,
    "/devices": spa,
    "/devices/*": spa,
    "/workspaces": spa,
    "/workspaces/*": spa,
    "/account": spa,
    "/api": spa,
    ...(production ? {
      "/assets/*": (req: Request) => frontendAsset(import.meta.dir, new URL(req.url).pathname.slice("/assets/".length)),
    } : {}),
    "/setup/:token": setupRoute,
    "/install.sh": installScript,
    "/downloads/*": (req: Request) => download(new URL(req.url).pathname.slice("/downloads/".length)),
    "/api/v1/health": () => json({ ok: true, version: VERSION, agents: agentsCount() }),
    "/healthz": new Response("ok"),
    "/robots.txt": new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } }),
  },
  async fetch(req, server) {
    const url = new URL(req.url);
    try {
      if (url.pathname === "/api/v1/agent/ws") {
        const requestedDevice = url.searchParams.get("device") || "";
        if (!requestedDevice || !q("SELECT 1 FROM devices WHERE id=?").get(requestedDevice)) return fail("device not found", 404);
        const deviceId = await verifyAgent(url);
        if (!deviceId) return fail("invalid agent signature", 401);
        if (server.upgrade(req, { data: { kind: "agent", deviceId } })) return undefined;
        return fail("upgrade failed", 400);
      }
      if (url.pathname === "/api/v1/ws") {
        const origin = req.headers.get("origin");
        if (origin && origin !== url.origin && origin !== PUBLIC_URL) return fail("invalid origin", 403);
        const user = await auth(req);
        if (!user) return fail("authentication required", 401);
        if (server.upgrade(req, { data: { kind: "browser", userId: user.id } })) return undefined;
        return fail("upgrade failed", 400);
      }
      if (url.pathname.startsWith("/api/v1/")) return await elysia.handle(req);
      return fail("not found", 404);
    } catch (error) {
      console.error(error);
      return fail(error instanceof Error ? error.message : "internal error", 500);
    }
  },
  websocket: {
    open(ws) {
      if (ws.data.kind === "agent") agentSocketHandlers.open(ws as ServerWebSocket<AgentData>);
      else browserSocketHandlers.open(ws as ServerWebSocket<BrowserData>);
    },
    message(ws, raw) {
      if (ws.data.kind === "agent") agentSocketHandlers.message(ws as ServerWebSocket<AgentData>, raw as string | Uint8Array);
      else browserSocketHandlers.message(ws as ServerWebSocket<BrowserData>, raw as string | Uint8Array);
    },
    close(ws) {
      if (ws.data.kind === "agent") agentSocketHandlers.close(ws as ServerWebSocket<AgentData>);
      else browserSocketHandlers.close(ws as ServerWebSocket<BrowserData>);
    },
  },
});

setInterval(() => {
  q("DELETE FROM auth_sessions WHERE expires_at<?").run(now());
  q("DELETE FROM workspace_invites WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM enrollment_tokens WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM webauthn_challenges WHERE expires_at<?").run(now());
}, 60_000).unref();

console.log(`Relay ${PUBLIC_URL} listening on :${server.port}`);
