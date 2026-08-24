import { Elysia } from "elysia";
import { apiRoutes } from "./routes/api";
import { artifactRoutes } from "./routes/artifacts";
import { pageActions } from "./routes/page-actions";
import { pageRoutes } from "./routes/pages";
import { agentSocketRoute } from "./routes/websocket-agent";
import { browserSocketRoute } from "./routes/websocket-browser";

export const app = new Elysia({ name: "relay" })
  .use(pageRoutes)
  .use(pageActions)
  .use(artifactRoutes)
  .use(apiRoutes)
  .use(browserSocketRoute)
  .use(agentSocketRoute)
  .get("/healthz", () => "ok", { detail: { hide: true } })
  .get("/robots.txt", () => new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } }), { detail: { hide: true } });

export type App = typeof app;
