import { PUBLIC_URL, PORT, SETUP_TOKEN } from "./src/config";
import { now, q, sha } from "./src/db";
import { AgentData, recoverInterruptedJobs, verifyAgent, websocketHandlers } from "./src/gateway";
import { fail, setupCookie } from "./src/http-utils";
import { handleAPI } from "./src/router";
import { staticResponse } from "./src/static";

recoverInterruptedJobs();

const server = Bun.serve<AgentData>({
  port: PORT,
  hostname: "0.0.0.0",
  idleTimeout: 60,
  async fetch(req, server) {
    const url = new URL(req.url);
    try {
      if (req.method === "GET" && url.pathname === "/" && url.searchParams.has("setup")) {
        if ((q<any>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return Response.redirect(PUBLIC_URL + "/", 303);
        const token = url.searchParams.get("setup") || "";
        if (!SETUP_TOKEN || sha(token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
        return new Response(null, {
          status: 303,
          headers: { location: "/", "set-cookie": setupCookie(token), "cache-control": "no-store" },
        });
      }
      if (url.pathname === "/api/v1/agent/ws") {
        const deviceId = verifyAgent(url);
        if (!deviceId) return fail("invalid agent signature", 401);
        if (server.upgrade(req, { data: { kind: "agent", deviceId } })) return undefined;
        return fail("upgrade failed", 400);
      }
      if (url.pathname.startsWith("/api/v1/")) return await handleAPI(req, url);
      if (url.pathname === "/healthz") return new Response("ok");
      if (url.pathname === "/robots.txt") return new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } });
      return await staticResponse(req, url.pathname);
    } catch (error: any) {
      console.error(error);
      return fail(error?.message || "internal error", 500);
    }
  },
  websocket: websocketHandlers,
});

setInterval(() => {
  q("DELETE FROM auth_sessions WHERE expires_at<?").run(now());
  q("DELETE FROM workspace_invites WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM enrollment_tokens WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM webauthn_challenges WHERE expires_at<?").run(now());
}, 60_000).unref();

console.log(`Relay ${PUBLIC_URL} listening on :${server.port}`);
