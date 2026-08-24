import { Elysia } from "elysia";
import { download, frontendAsset, installScript } from "../artifacts";

const production = Bun.env.NODE_ENV === "production";

export const artifactRoutes = new Elysia({ name: "relay.artifacts" })
  .get("/install.sh", installScript)
  .get("/downloads/*", ({ params }) => download(params["*"]))
  .get("/assets/*", ({ params }) => production ? frontendAsset(import.meta.dir, params["*"]) : new Response("not found", { status: 404 }));
