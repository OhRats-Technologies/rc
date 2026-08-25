import { Elysia } from "elysia";
import { apiRoutes } from "./routes/api";
import { artifactRoutes } from "./routes/artifacts";
import { pageActions } from "./routes/page-actions";
import { pageRoutes } from "./routes/pages";
import { agentSocketRoute } from "./routes/websocket-agent";
import { browserSocketRoute } from "./routes/websocket-browser";
import { mcpRoutes } from "./routes/mcp";
import { oauthRoutes } from "./routes/oauth";
import { pageContext } from "./page-context";
import { notFoundPage } from "../web/server/pages/auth";
import { security } from "./security";

export const app = new Elysia({ name: "rc" })
  .use(security)
  .use(pageRoutes)
  .use(pageActions)
  .use(artifactRoutes)
  .use(apiRoutes)
  .use(oauthRoutes)
  .use(mcpRoutes)
  .use(browserSocketRoute)
  .use(agentSocketRoute)
  .get("/healthz", () => "ok", { detail: { hide: true } })
  .get("/robots.txt", () => new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } }), { detail: { hide: true } })
  .all("/*", async ({ request }) => {
    const path = new URL(request.url).pathname;
    if (path.startsWith("/api/") || path.startsWith("/assets/") || path.startsWith("/downloads/") || path.startsWith("/oauth/") || path === "/mcp" || path.startsWith("/.well-known/")) {
      return new Response("not found", { status: 404 });
    }
    const context = await pageContext(request);
    return context ? notFoundPage(context.user, context.workspaces, context.sidebar) : notFoundPage();
  }, { detail: { hide: true } });

export type App = typeof app;
