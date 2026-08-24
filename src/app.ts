import { Elysia } from "elysia";
import { apiRoutes } from "./routes/api";
import { artifactRoutes } from "./routes/artifacts";
import { spaRoutes } from "./routes/pages-spa";
import { agentSocketRoute } from "./routes/websocket-agent";
import { browserSocketRoute } from "./routes/websocket-browser";

export const app = new Elysia({ name: "relay" })
  .use(spaRoutes)
  .use(artifactRoutes)
  .use(apiRoutes)
  .use(browserSocketRoute)
  .use(agentSocketRoute)
  .get("/healthz", () => "ok")
  .get("/robots.txt", () => new Response("User-agent: *\nDisallow: /\n", { headers: { "content-type": "text/plain" } }));

export type App = typeof app;
